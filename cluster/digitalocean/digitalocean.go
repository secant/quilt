package digitalocean

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/digitalocean/godo"

	"github.com/NetSys/quilt/cluster/acl"
	"github.com/NetSys/quilt/cluster/cloudcfg"
	"github.com/NetSys/quilt/cluster/machine"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/util"

	"golang.org/x/oauth2"

	log "github.com/Sirupsen/logrus"
)

// Digital Ocean Setup
// 1. Create a new key at cloud.digitalocean.com/settings/api/tokens
// 2. Copy and paste the key into this file: ~/.digitalocean/key

// DefaultRegion is assigned to Machines without a specified region
const DefaultRegion string = "nyc1"

var apiKeyPath = ".digitalocean/key"

var image = godo.DropletCreateImage{
	Slug: "ubuntu-16-04-x64",
}

// The DoCluster object represents a connection to DigitalOcean.
type DoCluster struct {
	client    client
	namespace string
}

type doClient struct {
	droplets       godo.DropletsService
	images         godo.ImagesService
	storage        godo.StorageService
	storageActions godo.StorageActionsService
}

type predicate func() bool

type tokenSource struct {
	AccessToken string
}

// Token generates an oauth2 token from the DigitalOcean API key.
func (t *tokenSource) Token() (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken: t.AccessToken,
	}
	return token, nil
}

// New starts a new client session with the API key provided in ~/.digitalocean/key.
func New(namespace string) (*DoCluster, error) {
	clst, err := newDigitalOcean(namespace)
	if err != nil {
		return clst, err
	}

	opt := &godo.ListOptions{}
	_, _, err = clst.client.ListDroplets(opt)

	return clst, err
}

// Creation is broken out for unit testing.
var newDigitalOcean = func(namespace string) (*DoCluster, error) {
	// Underscores are illegal. Uppercase is illegal in volume names.
	if strings.Contains(namespace, "-") || strings.Contains(namespace, "_") {
		log.Warning("DigitalOcean namespaces cannot have underscores, hypens, " +
			"or uppercase letters")
	}
	namespace = strings.ToLower(strings.Replace(namespace, "_", "-", -1))
	keyFile := filepath.Join(os.Getenv("HOME"), apiKeyPath)
	dat, err := util.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	apiKey := strings.TrimSpace(dat)
	tokenSrc := &tokenSource{
		AccessToken: apiKey,
	}
	oauthClient := oauth2.NewClient(oauth2.NoContext, tokenSrc)
	api := godo.NewClient(oauthClient)

	clst := &DoCluster{
		namespace: namespace,
		client: doClient{
			droplets:       api.Droplets,
			images:         api.Images,
			storage:        api.Storage,
			storageActions: api.StorageActions,
		},
	}
	return clst, nil
}

// List will fetch all droplets that have the same name as the cluster namespace.
func (clst DoCluster) List() ([]machine.Machine, error) {
	machines := []machine.Machine{}
	opt := &godo.ListOptions{} // Keep track of the page we're on.

	// DigitalOcean's API has a paginated list of droplets.
	for {
		droplets, resp, err := clst.client.ListDroplets(opt)
		if err != nil {
			return nil, err
		}

		for _, d := range droplets {
			if d.Name != clst.namespace {
				continue
			}

			pubIP, err := d.PublicIPv4()
			if err != nil {
				return nil, err
			}
			if pubIP == "" {
				return nil, fmt.Errorf("droplet %d has no public IP",
					d.ID)
			}

			privIP, err := d.PrivateIPv4()
			if err != nil {
				return nil, err
			}
			if privIP == "" {
				return nil, fmt.Errorf("droplet %d has no private IP",
					d.ID)
			}

			if len(d.VolumeIDs) == 0 {
				return nil, fmt.Errorf("droplet %d has no attached "+
					"volume", d.ID)
			}

			volume, _, err := clst.client.GetVolume(d.VolumeIDs[0])
			if err != nil {
				return nil, err
			}

			machine := machine.Machine{
				ID:        strconv.Itoa(d.ID),
				PublicIP:  pubIP,
				PrivateIP: privIP,
				Size:      d.SizeSlug,
				DiskSize:  int(volume.SizeGigaBytes),
				Provider:  db.DigitalOcean,
				Region:    d.Region.Slug,
			}
			machines = append(machines, machine)
		}

		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}

		opt.Page = page + 1
	}
	return machines, nil
}

// Boot will boot every machine in a goroutine, and wait for the machines to come up.
func (clst DoCluster) Boot(bootSet []machine.Machine) error {
	if len(bootSet) <= 0 {
		return nil
	}

	var wg sync.WaitGroup
	// If any calls to createAndAttach fail, one of the errors will be fed to this
	// channel.
	errChan := make(chan error, 1)
	for _, m := range bootSet {
		wg.Add(1)
		go func(m machine.Machine) {
			defer wg.Done()
			if err := clst.createAndAttach(m); err != nil {
				log.WithError(err).Warning("Failed to create droplet")
				select {
				case errChan <- err:
				default:
				}
			}

		}(m)
	}
	wg.Wait()

	var err error
	select {
	case err = <-errChan:
	default:
	}
	return err
}

func (clst DoCluster) machineUp(id int) predicate {
	return func() bool {
		d, _, _ := clst.client.GetDroplet(id)
		return d.Status == "active"
	}
}

func (clst DoCluster) volumeUp(id int) predicate {
	return func() bool {
		d, _, _ := clst.client.GetDroplet(id)
		return len(d.VolumeIDs) > 0
	}
}

// Creates a new machine, and waits for the machine to become active.
func (clst DoCluster) createAndAttach(m machine.Machine) error {
	/*
		if len(machine.FloatingIP) > 0 {
			panic("DigitalOcean floating IPs are unimplemented")
		}
	*/
	cloudConfig := cloudcfg.Ubuntu(m.SSHKeys, "xenial")
	createReq := &godo.DropletCreateRequest{
		Name:              clst.namespace,
		Region:            m.Region,
		Size:              m.Size,
		Image:             image,
		PrivateNetworking: true,
		UserData:          cloudConfig,
	}

	d, _, err := clst.client.CreateDroplet(createReq)
	if err != nil {
		return err
	}

	err = util.WaitFor(clst.machineUp(d.ID), 10*time.Second, 2*time.Minute)
	if err != nil {
		return err
	}

	volReq := &godo.VolumeCreateRequest{
		Region:        m.Region,
		Name:          fmt.Sprintf("quilt-%d-%s", d.ID, clst.namespace),
		SizeGigaBytes: int64(m.DiskSize),
	}
	v, _, err := clst.client.CreateVolume(volReq)
	if err != nil {
		return err
	}

	_, _, err = clst.client.AttachVolume(v.ID, d.ID)
	if err != nil {
		return err
	}

	err = util.WaitFor(clst.volumeUp(d.ID), 5*time.Second, 2*time.Minute)
	return err
}

// UpdateFloatingIPs currently returns an error.
func (clst DoCluster) UpdateFloatingIPs(machines []machine.Machine) error {
	return errors.New("digitalOcean floating IPs are unimplemented")
}

// Stop stops each machine and deletes their attached volumes.
func (clst DoCluster) Stop(machines []machine.Machine) error {
	if len(machines) <= 0 {
		return nil
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 1)
	for _, m := range machines {
		wg.Add(1)
		go func(m machine.Machine) {
			defer wg.Done()
			if err := clst.deleteAndWait(m.ID); err != nil {
				log.WithError(err).Warning("Failed to stop droplet")
				select {
				case errChan <- err:
				default:
				}
			}
		}(m)
	}

	wg.Wait()
	var err error
	select {
	case err = <-errChan:
	default:
	}

	return err
}

func (clst DoCluster) machineGone(id int) predicate {
	return func() bool {
		d, _, _ := clst.client.GetDroplet(id)
		return d == nil
	}
}

func (clst DoCluster) deleteAndWait(ids string) error {
	id, err := strconv.Atoi(ids)
	if err != nil {
		return err
	}
	d, _, _ := clst.client.GetDroplet(id)
	volIds := d.VolumeIDs
	_, err = clst.client.DeleteDroplet(id)
	if err != nil {
		return err
	}

	err = util.WaitFor(clst.machineGone(id), 500*time.Millisecond, 1*time.Minute)
	if err != nil {
		return err
	}

	for _, vol := range volIds {
		if _, err := clst.client.DeleteVolume(vol); err != nil {
			return err
		}
	}
	return nil
}

// SetACLs is not supported in DigitalOcean.
func (clst DoCluster) SetACLs(acls []acl.ACL) error {
	return errors.New("digitalocean does not support setting ACLs")
}

// Wrapper functions for DigitalOcean API.
func (client doClient) CreateDroplet(req *godo.DropletCreateRequest) (*godo.Droplet,
	*godo.Response, error) {
	return client.droplets.Create(req)
}

func (client doClient) DeleteDroplet(id int) (*godo.Response, error) {
	return client.droplets.Delete(id)
}

func (client doClient) GetDroplet(id int) (*godo.Droplet, *godo.Response, error) {
	return client.droplets.Get(id)
}

func (client doClient) ListDroplets(opt *godo.ListOptions) ([]godo.Droplet,
	*godo.Response, error) {
	return client.droplets.List(opt)
}

func (client doClient) CreateVolume(req *godo.VolumeCreateRequest) (*godo.Volume,
	*godo.Response, error) {
	return client.storage.CreateVolume(req)
}

func (client doClient) DeleteVolume(id string) (*godo.Response, error) {
	return client.storage.DeleteVolume(id)
}

func (client doClient) GetVolume(id string) (*godo.Volume, *godo.Response, error) {
	return client.storage.GetVolume(id)
}

func (client doClient) AttachVolume(vID string, dID int) (*godo.Action, *godo.Response,
	error) {
	return client.storageActions.Attach(vID, dID)
}
