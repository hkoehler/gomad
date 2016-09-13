// define different kinds of HTTP request handlers
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// Registry entry is HTTP handler with path
type Handler interface {
	http.Handler
	Path() string
	Name() string
}

// Common implementation of registry entry for commands
type HandlerImpl struct {
	path string
	name string
}

func (entry HandlerImpl) Path() string {
	return entry.path
}

func (entry HandlerImpl) Name() string {
	return entry.name
}

// HTTP handler executing command line
type CommandHandler struct {
	HandlerImpl
	CmdLine string
}

func NewCommandHandler(conf HandlerConfig) Handler {
	if conf.Type == "" {
		conf.Type = "command"
	}
	switch strings.ToLower(conf.Type) {
	case "command":
		return &CommandHandler{HandlerImpl: HandlerImpl{conf.URL, conf.Name},
			CmdLine: conf.Cmd}
	default:
		log.Fatal(fmt.Sprintf("Unknown handler type %s", conf.Type))
	}
	return nil
}

func (handler CommandHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	cmd := strings.Split(handler.CmdLine, " ")
	if out, err := exec.Command(cmd[0], cmd[1:]...).Output(); err == nil {
		fmt.Fprint(w, string(out))
	} else {
		fmt.Fprintf(w, "Error executing command line \"%s\": %v\n", handler.CmdLine, err)
	}
}

// HTTP handler displaying config
type ConfigHandler struct {
	HandlerImpl
	ConfigPath string
}

func NewConfigHandler(path string, configPath string) Handler {
	return &ConfigHandler{HandlerImpl: HandlerImpl{path, "Config"},
		ConfigPath: configPath}
}

func (handler ConfigHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if f, err := os.Open(handler.ConfigPath); err == nil {
		io.Copy(w, f)
	} else {
		fmt.Fprintf(w, "Couldn't open %s: %v", handler.Path, err)
	}
}
