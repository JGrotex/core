package engine

import (
	"fmt"
	"strings"

	"github.com/project-flogo/core/action"
	"github.com/project-flogo/core/app"
	"github.com/project-flogo/core/engine/channels"
	"github.com/project-flogo/core/engine/runner"
	"github.com/project-flogo/core/engine/secret"
	"github.com/project-flogo/core/support"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/core/support/managed"
)

// engineImpl is the type for the Default Engine Implementation
type engineImpl struct {
	config         *Config
	flogoApp       *app.App
	actionRunner   action.Runner
	serviceManager *support.ServiceManager
	logger         log.Logger
}

type Option func(*engineImpl)

// New creates a new Engine
func New(appConfig *app.Config, options... Option) (Engine, error) {
	if appConfig == nil {
		return nil, fmt.Errorf("no App configuration provided")
	}
	if len(appConfig.Name) == 0 {
		return nil, fmt.Errorf("no App name provided")
	}
	if len(appConfig.Version) == 0 {
		return nil, fmt.Errorf("no App version provided")
	}

	engine := &engineImpl{}
	logger := log.ChildLogger(log.RootLogger(), "engine")
	engine.logger = logger
	//log.SetLogLevel(log.DebugLevel, logger)

	if engine.config == nil {
		config := &Config{}
		config.StopEngineOnError = true
		config.RunnerType = GetRunnerType()
		//config.LogLevel = DefaultLogLevel

		engine.config = config
	}

	for _, option := range options {
		option(engine)
	}

	//add engine channels - todo should these be moved to app
	channelDescriptors := appConfig.Channels
	if len(channelDescriptors) > 0 {
		for _, descriptor := range channelDescriptors {
			name, buffSize := channels.Decode(descriptor)

			logger.Debugf("Creating Engine Channel '%s'", name)
			channels.New(name, buffSize)
		}
	}

	if engine.actionRunner == nil {
		var actionRunner action.Runner

		runnerType := engine.config.RunnerType
		if strings.EqualFold(ValueRunnerTypePooled, runnerType) {
			actionRunner = runner.NewPooled(NewPooledRunnerConfig())
		} else if strings.EqualFold(ValueRunnerTypeDirect, runnerType) {
			actionRunner = runner.NewDirect()
		} else {
			return nil, fmt.Errorf("unknown runner type: %s", runnerType)
		}

		logger.Debugf("Using '%s' Action Runner", runnerType)

		engine.actionRunner = actionRunner
	}

	var appOptions []app.Option
	if !engine.config.StopEngineOnError {
		appOptions = append(appOptions, app.ContinueOnError)
	}

	propProvider := GetAppPropertyProvider()
	propOverride := GetAppPropertyOverride()

	if len(propOverride) > 0 {
		option := app.ExternalProperties(propProvider, propOverride, secret.PropertyProcessor, EnvPropertyProcessor)
		appOptions = append(appOptions, option)
	}

	flogoApp, err := app.New(appConfig, engine.actionRunner, appOptions...)
	if err != nil {
		return nil, err
	}

	logger.Debugf("Creating app [ %s ] with version [ %s ]", appConfig.Name, appConfig.Version)
	engine.flogoApp = flogoApp
	engine.serviceManager = support.GetDefaultServiceManager()

	return engine, nil
}

func (e *engineImpl) App() *app.App {
	return e.flogoApp
}

//Start initializes and starts the Triggers and initializes the Actions
func (e *engineImpl) Start() error {

	logger := e.logger

	logger.Infof("Starting app [ %s ] with version [ %s ]", e.flogoApp.Name(), e.flogoApp.Version())

	logger.Info("Engine Starting...")

	logger.Info("Starting Services...")

	actionRunner := e.actionRunner.(interface{})

	if managedRunner, ok := actionRunner.(managed.Managed); ok {
		managed.Start("ActionRunner Service", managedRunner)
	}

	err := e.serviceManager.Start()

	if err != nil {
		logger.Error("Error Starting Services - " + err.Error())
	} else {
		logger.Info("Started Services")
	}

	if len(managedServices) > 0 {
		for _, mService := range managedServices {
			err = mService.Start()
			if err != nil {
				logger.Error("Error Starting Services - " + err.Error())
				//TODO Should we exit here?
			}
		}
	}

	logger.Info("Starting Application...")
	err = e.flogoApp.Start()
	if err != nil {
		return err
	}
	logger.Info("Application Started")

	if channels.Count() > 0 {
		logger.Info("Starting Engine Channels...")
		channels.Start()
		logger.Info("Engine Channels Started")
	}

	logger.Info("Engine Started")

	return nil
}

func (e *engineImpl) Stop() error {

	logger := e.logger

	logger.Info("Engine Stopping...")

	if channels.Count() > 0 {
		logger.Info("Stopping Engine Channels...")
		channels.Stop()
		logger.Info("Engine Channels Stopped...")
	}

	logger.Info("Stopping Application...")
	e.flogoApp.Stop()
	logger.Info("Application Stopped")

	//TODO temporarily add services
	logger.Info("Stopping Services...")

	actionRunner := e.actionRunner.(interface{})

	if managedRunner, ok := actionRunner.(managed.Managed); ok {
		managed.Stop("ActionRunner", managedRunner)
	}

	err := e.serviceManager.Stop()

	if err != nil {
		logger.Error("Error Stopping Services - " + err.Error())
	} else {
		logger.Info("Stopped Services")
	}

	if len(managedServices) > 0 {
		for _, mService := range managedServices {
			err = mService.Stop()
			if err != nil {
				logger.Error("Error Stopping Services - " + err.Error())
			}
		}
	}

	logger.Info("Engine Stopped")
	log.Sync()

	return nil
}
