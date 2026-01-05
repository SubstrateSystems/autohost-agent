package docker

import "autohost-agent/internal/domain"

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Install() error                         { return Install() }
func (a *Adapter) StopApp(appName domain.AppName) error   { return Stop(appName) }
func (a *Adapter) StartApp(appName domain.AppName) error  { return Start(appName) }
func (a *Adapter) RemoveApp(appName domain.AppName) error { return Remove(appName) }
func (a *Adapter) GetAppStatus(appName domain.AppName) (string, error) {
	return GetAppStatus(appName)
}
func (a *Adapter) DockerInstalled() bool       { return DockerInstalled() }
func (a *Adapter) CreateDockerNetwork() error  { return CreateDockerNetwork() }
func (a *Adapter) AddUserToDockerGroup() error { return AddUserToDockerGroup() }
