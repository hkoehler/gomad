// Copyright (C) 2016, Heiko Koehler
// define different kinds of HTTP request handlers
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"regexp"
	"errors"
	_ "log"
)

type HandlerConfig struct {
	Type string
	Name string
	Cmd  string
	URL  string
	Regex string
	Submatches []string
}

// Registry entry is HTTP handler with path
type Handler interface {
	http.Handler
	// return raw and key-val/parsed output
	Stat() (string, map[string]string)
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
	Regex *regexp.Regexp
	Submatches []string
}

func NewCommandHandler(conf HandlerConfig) (Handler, error) {
	if re, err := regexp.Compile(conf.Regex); err != nil {
		return nil, err
	} else {
		return &CommandHandler{HandlerImpl: HandlerImpl{conf.URL, conf.Name},
			CmdLine: conf.Cmd, Regex : re, Submatches : conf.Submatches}, nil
	}
}

func NewHandler(conf HandlerConfig) (Handler, error) {
	if conf.Type == "" {
		conf.Type = "command"
	}
	switch strings.ToLower(conf.Type) {
	case "command":
		return NewCommandHandler(conf)
	}
	return nil, errors.New(fmt.Sprintf("Unknown handler type %s", conf.Type))
}

func (handler CommandHandler) Stat() (string, map[string]string) {
	cmd := strings.Split(handler.CmdLine, " ")
	if cmdOut, err := exec.Command(cmd[0], cmd[1:]...).Output(); err == nil {
		var attrs map[string]string
		
		out := string(cmdOut)
		out += "\n"
		if handler.Regex.MatchString(out) {
			attrs := handler.Regex.FindStringSubmatch(out)
			attrs = attrs[1:]
			for i, m := range attrs {
				out += fmt.Sprintf("\"%s\" = \"%s\"\n", handler.Submatches[i], m)
			}
		}
		return out, attrs
	} else {
		return fmt.Sprintf("Error executing command line \"%s\": %v\n", handler.CmdLine, err), nil
	}
}

func (handler CommandHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	out, attrs := handler.Stat()
	_ = attrs
	fmt.Fprint(w, out)
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

func (handler ConfigHandler) Stat() (string, map[string]string) {
	return "", map[string]string{}
}

func (handler ConfigHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if f, err := os.Open(handler.ConfigPath); err == nil {
		io.Copy(w, f)
	} else {
		fmt.Fprintf(w, "Couldn't open %s: %v", handler.Path, err)
	}
}
