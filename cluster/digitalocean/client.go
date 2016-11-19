package digitalocean

import "github.com/digitalocean/godo"

// client for DigitalOcean's API. Used for unit testing.
type client interface {
	CreateDroplet(*godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error)
	DeleteDroplet(int) (*godo.Response, error)
	GetDroplet(int) (*godo.Droplet, *godo.Response, error)
	ListDroplets(*godo.ListOptions) ([]godo.Droplet, *godo.Response, error)
	GetVolume(string) (*godo.Volume, *godo.Response, error)
	CreateVolume(*godo.VolumeCreateRequest) (*godo.Volume, *godo.Response, error)
	AttachVolume(string, int) (*godo.Action, *godo.Response, error)
	DeleteVolume(string) (*godo.Response, error)
}
