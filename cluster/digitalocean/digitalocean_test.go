//go:generate mockery -inpkg -name=client
package digitalocean

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/digitalocean/godo"

	"github.com/spf13/afero"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/NetSys/quilt/cluster/acl"
	"github.com/NetSys/quilt/cluster/machine"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/util"
)

const testNamespace = "namespace"
const errMsg = "error"

var errMock = errors.New(errMsg)

var network = &godo.Networks{
	V4: []godo.NetworkV4{
		{
			IPAddress: "privateIP",
			Netmask:   "255.255.255.255",
			Gateway:   "2.2.2.2",
			Type:      "private",
		},
		{
			IPAddress: "publicIP",
			Netmask:   "255.255.255.255",
			Gateway:   "2.2.2.2",
			Type:      "public",
		},
	},
}

var nyc = &godo.Region{
	Slug: "nyc1",
}

func init() {
	util.AppFs = afero.NewMemMapFs()
	keyFile := filepath.Join(os.Getenv("HOME"), apiKeyPath)
	util.WriteFile(keyFile, []byte("foo"), 0666)
}

func TestList(t *testing.T) {
	mc := new(mockClient)
	// Create a list of Droplets, that are paginated.
	dropFirst := []godo.Droplet{
		{
			ID:        123,
			Name:      testNamespace,
			Networks:  network,
			SizeSlug:  "size",
			VolumeIDs: []string{"foo"},
			Region:    nyc,
		},

		// This droplet should not be listed because it has a name different from
		// testNamespace.
		{
			ID:        124,
			Name:      "foo",
			Networks:  network,
			SizeSlug:  "size",
			VolumeIDs: []string{"foo"},
			Region:    nyc,
		},
	}

	dropLast := []godo.Droplet{
		{
			ID:        125,
			Name:      testNamespace,
			Networks:  network,
			SizeSlug:  "size",
			VolumeIDs: []string{"foo"},
			Region:    nyc,
		},
	}

	respFirst := &godo.Response{
		Links: &godo.Links{
			Pages: &godo.Pages{
				Last: "2",
			},
		},
	}

	respLast := &godo.Response{
		Links: &godo.Links{},
	}

	reqFirst := &godo.ListOptions{}
	mc.On("ListDroplets", reqFirst).Return(dropFirst, respFirst, nil).Once()

	reqLast := &godo.ListOptions{
		Page: reqFirst.Page + 1,
	}
	mc.On("ListDroplets", reqLast).Return(dropLast, respLast, nil).Once()

	mc.On("GetVolume", mock.Anything).Return(
		&godo.Volume{
			SizeGigaBytes: 32,
		}, nil, nil,
	).Twice()

	doClust, err := newDigitalOcean(testNamespace)
	assert.Nil(t, err)
	doClust.client = mc

	machines, err := doClust.List()
	assert.Nil(t, err)
	assert.Equal(t, machines, []machine.Machine{
		{
			ID:        "123",
			Provider:  db.DigitalOcean,
			PublicIP:  "publicIP",
			PrivateIP: "privateIP",
			Size:      "size",
			DiskSize:  32,
			Region:    "nyc1",
		},
		{
			ID:        "125",
			Provider:  db.DigitalOcean,
			PublicIP:  "publicIP",
			PrivateIP: "privateIP",
			Size:      "size",
			DiskSize:  32,
			Region:    "nyc1",
		},
	})

	// Error ListDroplets.
	mc.On("ListDroplets", mock.Anything).Return(nil, nil, errMock).Once()
	machines, err = doClust.List()
	assert.Nil(t, machines)
	assert.EqualError(t, err, errMsg)

	// Error PublicIPv4. We can't error PrivateIPv4 because of the two functions'
	// similarities and the order that they are called in `List`.
	droplets := []godo.Droplet{
		{
			ID:        123,
			Name:      testNamespace,
			Networks:  nil,
			SizeSlug:  "size",
			VolumeIDs: []string{"foo"},
			Region:    nyc,
		},
	}
	mc.On("ListDroplets", mock.Anything).Return(droplets, respLast, nil).Once()
	machines, err = doClust.List()
	assert.Nil(t, machines)
	assert.EqualError(t, err, "no networks have been defined")

	droplets[0].Networks = &godo.Networks{
		V4: []godo.NetworkV4{
			{
				IPAddress: "privateIP",
				Netmask:   "255.255.255.255",
				Gateway:   "2.2.2.2",
				Type:      "private",
			},
		},
	}
	mc.On("ListDroplets", mock.Anything).Return(droplets, respLast, nil).Once()
	machines, err = doClust.List()
	assert.Nil(t, machines)
	assert.EqualError(t, err, "droplet 123 has no public IP")

	droplets[0].Networks.V4[0].Type = "public"
	mc.On("ListDroplets", mock.Anything).Return(droplets, respLast, nil).Once()
	machines, err = doClust.List()
	assert.Nil(t, machines)
	assert.EqualError(t, err, "droplet 123 has no private IP")

	droplets[0].Networks = network
	droplets[0].VolumeIDs = []string{}
	mc.On("ListDroplets", mock.Anything).Return(droplets, respLast, nil).Once()
	machines, err = doClust.List()
	assert.Nil(t, machines)
	assert.EqualError(t, err, "droplet 123 has no attached volume")

	droplets[0].VolumeIDs = []string{"foo"}
	mc.On("ListDroplets", mock.Anything).Return(droplets, respLast, nil).Once()
	mc.On("GetVolume", mock.Anything).Return(nil, nil, errMock)
	machines, err = doClust.List()
	assert.Nil(t, machines)
	assert.EqualError(t, err, errMsg)

	respBad := &godo.Response{
		Links: &godo.Links{
			Pages: &godo.Pages{
				Prev: "badurl",
				Last: "2",
			},
		},
	}
	mc.On("ListDroplets", mock.Anything).Return([]godo.Droplet{}, respBad, nil).Once()
	machines, err = doClust.List()
	assert.Nil(t, machines)
	assert.EqualError(t, err, "parse badurl: invalid URI for request")
}

func TestBoot(t *testing.T) {
	mc := new(mockClient)
	doClust, err := newDigitalOcean(testNamespace)
	assert.Nil(t, err)
	doClust.client = mc

	util.Sleep = func(t time.Duration) {}

	bootSet := []machine.Machine{}
	err = doClust.Boot(bootSet)
	assert.Nil(t, err)

	// Create a list of machines to boot.
	bootSet = []machine.Machine{
		{
			ID:        "123",
			Provider:  db.DigitalOcean,
			PublicIP:  "publicIP",
			PrivateIP: "privateIP",
			Size:      "size",
			DiskSize:  32,
			Region:    "nyc1",
		},
	}

	mc.On("GetDroplet", 123).Return(&godo.Droplet{
		Status:    "active",
		VolumeIDs: []string{"abc"},
	}, nil, nil).Twice()

	mc.On("CreateDroplet", mock.Anything).Return(&godo.Droplet{
		ID: 123,
	}, nil, nil).Once()

	mc.On("CreateVolume", mock.Anything).Return(&godo.Volume{
		ID: "abc",
	}, nil, nil).Once()

	mc.On("AttachVolume", mock.Anything, mock.Anything).Return(nil, nil, nil).Once()

	err = doClust.Boot(bootSet)
	// Make sure machines are booted.
	mc.AssertNumberOfCalls(t, "CreateDroplet", 1)
	mc.AssertNumberOfCalls(t, "CreateVolume", 1)
	assert.Nil(t, err)

	// Error CreateDroplet.
	doubleBootSet := append(bootSet, machine.Machine{
		ID:        "123",
		Provider:  db.DigitalOcean,
		PublicIP:  "publicIP",
		PrivateIP: "privateIP",
		Size:      "size",
		DiskSize:  32,
		Region:    "nyc1",
	})
	mc.On("CreateDroplet", mock.Anything).Return(nil, nil, errMock).Twice()
	err = doClust.Boot(doubleBootSet)
	assert.EqualError(t, err, errMsg)

	// Error CreateVolume
	mc.On("GetDroplet", 123).Return(&godo.Droplet{
		Status:    "active",
		VolumeIDs: []string{"abc"},
	}, nil, nil).Once()

	mc.On("CreateDroplet", mock.Anything).Return(&godo.Droplet{
		ID: 123,
	}, nil, nil).Once()

	mc.On("CreateVolume", mock.Anything).Return(nil, nil, errMock).Once()
	err = doClust.Boot(bootSet)
	assert.EqualError(t, err, errMsg)

	// Error AttachVolume
	mc.On("GetDroplet", 123).Return(&godo.Droplet{
		Status:    "active",
		VolumeIDs: []string{"abc"},
	}, nil, nil).Once()

	mc.On("CreateDroplet", mock.Anything).Return(&godo.Droplet{
		ID: 123,
	}, nil, nil).Once()

	mc.On("CreateVolume", mock.Anything).Return(&godo.Volume{
		ID: "abc",
	}, nil, nil).Once()

	mc.On("AttachVolume", mock.Anything, mock.Anything).Return(nil, nil,
		errMock).Once()
	err = doClust.Boot(bootSet)
	assert.EqualError(t, err, errMsg)

	// Test time-out.
	i := 0
	util.After = func(t time.Time) bool {
		i++
		return i >= 1
	}

	mc.On("CreateDroplet", mock.Anything).Return(&godo.Droplet{
		ID: 123,
	}, nil, nil).Once()
	mc.On("GetDroplet", 123).Return(&godo.Droplet{
		Status:    "not active",
		VolumeIDs: []string{"abc"},
	}, nil, nil).Twice()
	err = doClust.Boot(bootSet)
	assert.EqualError(t, err, "timed out")
}

func TestStop(t *testing.T) {
	mc := new(mockClient)
	doClust, err := newDigitalOcean(testNamespace)
	assert.Nil(t, err)
	doClust.client = mc

	util.Sleep = func(t time.Duration) {}

	// Test empty stop set
	stopSet := []machine.Machine{}
	err = doClust.Stop(stopSet)
	assert.Nil(t, err)

	// Test non-empty stop set
	stopSet = []machine.Machine{
		{
			ID:        "123",
			Provider:  db.DigitalOcean,
			PublicIP:  "publicIP",
			PrivateIP: "privateIP",
			Size:      "size",
			DiskSize:  32,
			Region:    "nyc1",
		},
	}

	mc.On("GetDroplet", 123).Return(&godo.Droplet{
		Status:    "active",
		VolumeIDs: []string{"abc"},
	}, nil, nil).Once()

	mc.On("GetDroplet", 123).Return(nil, nil, nil).Once()

	mc.On("DeleteDroplet", 123).Return(nil, nil).Once()

	mc.On("DeleteVolume", "abc").Return(nil, nil).Once()

	err = doClust.Stop(stopSet)

	// Make sure machines are stopped.
	mc.AssertNumberOfCalls(t, "GetDroplet", 2)
	mc.AssertNumberOfCalls(t, "DeleteVolume", 1)
	assert.Nil(t, err)

	// Error strconv.
	badDoubleStopSet := []machine.Machine{
		{
			ID:        "123a",
			Provider:  db.DigitalOcean,
			PublicIP:  "publicIP",
			PrivateIP: "privateIP",
			Size:      "size",
			DiskSize:  32,
			Region:    "nyc1",
		},
		{
			ID:        "123a",
			Provider:  db.DigitalOcean,
			PublicIP:  "publicIP",
			PrivateIP: "privateIP",
			Size:      "size",
			DiskSize:  32,
			Region:    "nyc1",
		},
	}
	err = doClust.Stop(badDoubleStopSet)
	assert.Error(t, err)

	// Error DeleteDroplet.
	mc.On("GetDroplet", 123).Return(&godo.Droplet{
		Status:    "active",
		VolumeIDs: []string{"abc"},
	}, nil, nil).Once()

	mc.On("DeleteDroplet", 123).Return(nil, errMock).Once()
	err = doClust.Stop(stopSet)
	assert.EqualError(t, err, errMsg)

	// Error DeleteVolume.
	mc.On("GetDroplet", 123).Return(&godo.Droplet{
		Status:    "active",
		VolumeIDs: []string{"abc"},
	}, nil, nil).Once()

	mc.On("GetDroplet", 123).Return(nil, nil, nil).Once()

	mc.On("DeleteDroplet", 123).Return(nil, nil).Once()

	mc.On("DeleteVolume", "abc").Return(nil, errMock).Once()
	err = doClust.Stop(stopSet)
	assert.EqualError(t, err, errMsg)

	// Test time-out.
	i := 0
	util.After = func(t time.Time) bool {
		i++
		return i >= 1
	}
	mc.On("GetDroplet", 123).Return(&godo.Droplet{
		Status:    "active",
		VolumeIDs: []string{"abc"},
	}, nil, nil).Times(3)

	mc.On("DeleteDroplet", 123).Return(nil, nil).Once()

	mc.On("DeleteVolume", "abc").Return(nil, errMock).Once()
	err = doClust.Stop(stopSet)
	assert.EqualError(t, err, "timed out")
}

func TestSetACLs(t *testing.T) {
	doClust, err := newDigitalOcean(testNamespace)
	assert.Nil(t, err)
	err = doClust.SetACLs([]acl.ACL{
		{
			CidrIP:  "digital",
			MinPort: 1,
			MaxPort: 65535,
		},
		{
			CidrIP:  "ocean",
			MinPort: 22,
			MaxPort: 22,
		},
	})

	assert.EqualError(t, err, "digitalocean does not support setting ACLs")
}

func TestUpdateFloatingIPs(t *testing.T) {
	doClust, err := newDigitalOcean(testNamespace)
	assert.Nil(t, err)
	err = doClust.UpdateFloatingIPs(nil)
	assert.EqualError(t, err, "digitalOcean floating IPs are unimplemented")
}

func TestNew(t *testing.T) {
	mc := new(mockClient)
	clust := &DoCluster{
		namespace: testNamespace,
		client:    mc,
	}

	// Log a bad namespace.
	newDigitalOcean("___ILLEGAL---")

	// newDigitalOcean throws an error.
	newDigitalOcean = func(namespace string) (*DoCluster, error) {
		return nil, errMock
	}
	outClust, err := New(testNamespace)
	assert.Nil(t, outClust)
	assert.EqualError(t, err, "error")

	// Normal operation.
	newDigitalOcean = func(namespace string) (*DoCluster, error) {
		return clust, nil
	}
	mc.On("ListDroplets", mock.Anything).Return(nil, nil, nil).Once()
	outClust, err = New(testNamespace)
	assert.Nil(t, err)
	assert.Equal(t, clust, outClust)

	// ListDroplets throws an error.
	mc.On("ListDroplets", mock.Anything).Return(nil, nil, errMock)
	outClust, err = New(testNamespace)
	assert.Equal(t, clust, outClust)
	assert.EqualError(t, err, errMsg)
}

func TestToken(t *testing.T) {
	tokenSrc := &tokenSource{
		AccessToken: "key",
	}
	token, err := tokenSrc.Token()
	assert.Nil(t, err)
	assert.True(t, token.Valid())
}
