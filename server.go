// Copyright 2015 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license. See the LICENSE file for details.
package gows

import (
	"net/http"
)

// Server is a managed HTTP server handling incoming connections to both application and admin.
type Server interface {
	Managed
}

// ServerHandler handles HTTP requests.
type ServerHandler interface {
	// Handle registers the handler for the given pattern.
	Handle(pattern string, handler http.Handler)
}

// ServerFactory builds Server with given configuration and environment.
type ServerFactory interface {
	BuildServer(configuration *Configuration, environment *Environment) (Server, error)
}

// DefaultServerConnector utilizes http.Server.
type DefaultServerConnector struct {
	Server        *http.Server
	configuration *ConnectorConfiguration
}

// newServerConnector allocates and returns a new DefaultServerConnector.
func newServerConnector(handler http.Handler, configuration *ConnectorConfiguration) *DefaultServerConnector {
	server := &http.Server{
		Addr:    configuration.Addr,
		Handler: handler,
	}
	connector := &DefaultServerConnector{
		Server:        server,
		configuration: configuration,
	}
	return connector
}

// start starts server connector.
func (connector *DefaultServerConnector) start() error {
	if connector.configuration.Type == "https" {
		return connector.Server.ListenAndServeTLS(connector.configuration.CertFile, connector.configuration.KeyFile)
	}
	return connector.Server.ListenAndServe()
}

// DefaultServer implements Server interface
type DefaultServer struct {
	Connectors []*DefaultServerConnector

	configuration *ServerConfiguration
}

// NewDefaultServer allocates and returns a new DefaultServer.
func NewDefaultServer(configuration *ServerConfiguration) *DefaultServer {
	return &DefaultServer{
		configuration: configuration,
	}
}

// Start starts all connectors of the server.
func (server *DefaultServer) Start() error {
	errorChan := make(chan error)

	for _, connector := range server.Connectors {
		go func(c *DefaultServerConnector) {
			errorChan <- c.start()
		}(connector)
	}
	for i := len(server.Connectors); i > 0; i-- {
		select {
		case err := <-errorChan:
			// TODO: stop server gratefully
			if err != nil {
				server.Stop()
				return err
			}
		}
	}
	return nil
}

// Start stops all running connectors of the server.
func (server *DefaultServer) Stop() error {
	// TODO
	return nil
}

// addConnectors adds a new connector to the server.
func (server *DefaultServer) AddConnectors(handler http.Handler, configurations []ConnectorConfiguration) {
	count := len(configurations)
	// Does "range" copy struct value?
	for i := 0; i < count; i++ {
		connector := newServerConnector(handler, &configurations[i])
		server.Connectors = append(server.Connectors, connector)
	}
}

// DefaultServerHandler implements ServerHandler and http.Handler interface.
type DefaultServerHandler struct {
	ContextPath string
	ServeMux    *http.ServeMux
}

// NewDefaultServerHandler allocates and returns a new DefaultServerHandler.
func NewDefaultServerHandler() *DefaultServerHandler {
	return &DefaultServerHandler{
		ServeMux: http.NewServeMux(),
	}
}

func (server *DefaultServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO: Add request and response filter
	server.ServeMux.ServeHTTP(w, r)
}

// Handle registers the handler for the given pattern.
func (server *DefaultServerHandler) Handle(pattern string, handler http.Handler) {
	path := server.ContextPath + pattern
	server.ServeMux.Handle(path, handler)
}

// DefaultServerFactory implements ServerFactory interface.
type DefaultServerFactory struct {
}

// BuildServer creates a new Server.
func (factory *DefaultServerFactory) BuildServer(configuration *Configuration, environment *Environment) (Server, error) {
	server := NewDefaultServer(&configuration.Server)
	// Application
	handler := NewDefaultServerHandler()
	server.AddConnectors(handler, server.configuration.ApplicationConnectors)
	environment.ServerHandler = handler
	// Admin
	handler = NewDefaultServerHandler()
	server.AddConnectors(handler, server.configuration.AdminConnectors)
	environment.Admin.ServerHandler = handler
	environment.Admin.Initialize(handler.ContextPath)
	return server, nil
}

// ServerCommand implements Command.
type ServerCommand struct {
}

// Name returns name of the ServerCommand.
func (command *ServerCommand) Name() string {
	return "server"
}

// Description returns description of the ServerCommand.
func (command *ServerCommand) Description() string {
	return "Runs the application as an HTTP server"
}

// Run runs the command with the given bootstrap.
func (command *ServerCommand) Run(bootstrap *Bootstrap) error {
	// Parse configuration
	configuration, err := bootstrap.ConfigurationFactory.BuildConfiguration(bootstrap.Arguments[1:])
	if err != nil {
		return err
	}
	// Create environment
	environment := NewEnvironment(bootstrap.Application.Name())
	server, err := bootstrap.ServerFactory.BuildServer(configuration, environment)
	if err != nil {
		return err
	}
	// Run all bundles in bootstrap
	if err = bootstrap.run(configuration, environment); err != nil {
		return err
	}
	// Run application
	if err = bootstrap.Application.Run(configuration, environment); err != nil {
		return err
	}
	return server.Start()
}
