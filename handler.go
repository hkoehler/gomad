// define different kinds of HTTP request handlers
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// Registry entry is HTTP handler with path
type RegistryEntry interface {
	http.Handler
	Path() string
	Name() string
}

// Common implementation of registry entry for commands
type RegistryEntryImpl struct {
	path string
	name string
}

func (entry RegistryEntryImpl) Path() string {
	return entry.path
}

func (entry RegistryEntryImpl) Name() string {
	return entry.name
}

// HTTP handler executing command line
type CommandHandler struct {
	RegistryEntryImpl
	CmdLine string
}

func NewCommandHandler(entry ConfigEntry) *CommandHandler {
	return &CommandHandler{RegistryEntryImpl: RegistryEntryImpl{entry.URL, entry.Name},
		CmdLine: entry.Cmd}
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
	RegistryEntryImpl
	ConfigPath string
}

func NewConfigHandler(path string, configPath string) *ConfigHandler {
	return &ConfigHandler{RegistryEntryImpl: RegistryEntryImpl{path, "Config"},
		ConfigPath: configPath}
}

func (handler ConfigHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if f, err := os.Open(handler.ConfigPath); err == nil {
		io.Copy(w, f)
	} else {
		fmt.Fprintf(w, "Couldn't open %s: %v", handler.Path, err)
	}
}
