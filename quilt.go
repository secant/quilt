package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	l_mod "log"
	"os"
	"strings"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/cluster"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/inspect"
	"github.com/NetSys/quilt/stitch"
	"github.com/NetSys/quilt/util"

	"google.golang.org/grpc/grpclog"

	log "github.com/Sirupsen/logrus"
)

func main() {
	/* XXX: GRPC spews a lot of uselss log message so we tell to eat its logs.
	 * Once we have more sophistcated logging support, we should enable the log
	 * messages when in debug mode. */
	grpclog.SetLogger(l_mod.New(ioutil.Discard, "", 0))

	log.SetFormatter(util.Formatter{})

	flag.Usage = func() {
		fmt.Println("Usage: quilt [inspect <stitch> | get <import_path>]" +
			" [-log-level=<level> | -l=<level>]")
		fmt.Println("\nWhen provided a stitch, quilt takes responsibility\n" +
			"for deploying it as specified.  Alternatively, quilt may be\n" +
			"instructed to stop all deployments in a given namespace,\n" +
			"or the default namespace if none is provided.\n")
		flag.PrintDefaults()
		fmt.Println("        Valid logger levels are:\n" +
			"            debug, info, warn, error, fatal or panic.")
	}

	var logLevel = flag.String("log-level", "info", "level to set logger to")
	flag.StringVar(logLevel, "l", "info", "level to set logger to")
	var lAddr = flag.String("H", api.DefaultSocket,
		"Socket to listen for API requests on.")
	flag.Parse()

	level, err := parseLogLevel(*logLevel)
	if err != nil {
		fmt.Println(err)
		usage()
	}
	log.SetLevel(level)

	conn := db.New()

	switch flag.Arg(0) {
	case "get":
		getSpec(flag.Arg(1))
	case "inspect":
		inspect.Main(flag.Args())
		return
	}

	go api.RunServer(conn, *lAddr)
	cluster.Run(conn)
}

func getSpec(importPath string) {
	if err := stitch.GetSpec(importPath); err != nil {
		log.Error(err)
	}

	os.Exit(0)
}

func usage() {
	flag.Usage()
	os.Exit(1)
}

// parseLogLevel returns the log.Level type corresponding to the given string
// (case insensitive).
// If no such matching string is found, it returns log.InfoLevel (default) and an error.
func parseLogLevel(logLevel string) (log.Level, error) {
	logLevel = strings.ToLower(logLevel)
	switch logLevel {
	case "debug":
		return log.DebugLevel, nil
	case "info":
		return log.InfoLevel, nil
	case "warn":
		return log.WarnLevel, nil
	case "error":
		return log.ErrorLevel, nil
	case "fatal":
		return log.FatalLevel, nil
	case "panic":
		return log.PanicLevel, nil
	}
	return log.InfoLevel, fmt.Errorf("bad log level: '%v'", logLevel)
}
