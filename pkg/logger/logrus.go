package logger

import (
	"fmt"
	"github.com/bombsimon/logrusr/v2"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

type LoggingConfig struct {
	Level string
	File  string
}

func ConfigureLogging(config *LoggingConfig) {
	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			fileName := path.Base(f.File)
			return fmt.Sprintf("%s()", f.Function), fmt.Sprintf("%s:%d", fileName, f.Line)
		},
	})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	//log.SetOutput(os.Stdout)
	log.SetReportCaller(true)

	lvl, ok := os.LookupEnv("LOG_LEVEL")
	// LOG_LEVEL not set, let's default to debug, log.WarnLevel
	if !ok {
		lvl = "debug"
	}
	// parse string, this is built-in feature of logrus
	ll, err := log.ParseLevel(lvl)
	if err != nil {
		ll = log.DebugLevel
	}
	// set global log level
	log.SetLevel(ll)

}

func ConfigControllerLog() {
	var logrusLog = log.New()
	logrusLog.SetLevel(log.TraceLevel)
	logrusLog.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			fileName := path.Base(f.File)
			return fmt.Sprintf("%s()", f.Function), fmt.Sprintf("%s:%d", fileName, f.Line)
		},
	})
	var log = logrusr.New(
		logrusLog,
		logrusr.WithReportCaller(),
	).WithCallDepth(0)
	ctrl.SetLogger(log)
}
