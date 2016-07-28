package api

import (
	"net"
	"time"

	"github.com/NetSys/quilt/api/pb"
	"github.com/NetSys/quilt/api/util"
	"github.com/NetSys/quilt/db"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// Client provides methods to interact with the Quilt daemon.
type Client struct {
	pbClient pb.APIClient
}

// NewClient creates a new Quilt client connected to `lAddr`.
func NewClient(lAddr string) (Client, error) {
	proto, addr, err := util.ParseListenAddress(lAddr)
	if err != nil {
		return Client{}, err
	}

	dialer := func(dialAddr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout(proto, dialAddr, timeout)
	}
	cc, err := grpc.Dial(addr, grpc.WithDialer(dialer), grpc.WithInsecure())
	if err != nil {
		return Client{}, err
	}

	pbClient := pb.NewAPIClient(cc)
	return Client{pbClient: pbClient}, nil
}

// QueryMachines retrieves the machines tracked by the Quilt daemon.
func (c Client) QueryMachines() ([]db.Machine, error) {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	reply, err := c.pbClient.QueryMachines(ctx, &pb.DBQuery{})
	if err != nil {
		return []db.Machine{}, err
	}

	var dbMachines []db.Machine
	for _, pbMachine := range reply.Machines {
		dbMachines = append(dbMachines, convertMachine(*pbMachine))
	}

	return dbMachines, nil
}

func convertMachine(pbMachine pb.Machine) db.Machine {
	return db.Machine{
		ID:        int(pbMachine.ID),
		Role:      db.Role(pbMachine.Role),
		Provider:  db.Provider(pbMachine.Provider),
		Region:    pbMachine.Region,
		Size:      pbMachine.Size,
		DiskSize:  int(pbMachine.DiskSize),
		SSHKeys:   pbMachine.SSHKeys,
		CloudID:   pbMachine.CloudID,
		PublicIP:  pbMachine.PublicIP,
		PrivateIP: pbMachine.PrivateIP,
		Connected: pbMachine.Connected,
	}
}

// QueryContainers retrieves the containers tracked by the Quilt daemon.
func (c Client) QueryContainers() ([]db.Container, error) {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	reply, err := c.pbClient.QueryContainers(ctx, &pb.DBQuery{})
	if err != nil {
		return []db.Container{}, err
	}

	var dbContainers []db.Container
	for _, pbContainer := range reply.Containers {
		dbContainers = append(dbContainers, convertContainer(*pbContainer))
	}

	return dbContainers, nil
}

func convertContainer(pbContainer pb.Container) db.Container {
	return db.Container{
		ID:       int(pbContainer.ID),
		DockerID: pbContainer.DockerID,
		Image:    pbContainer.Image,
		Command:  pbContainer.Command,
		Labels:   pbContainer.Labels,
	}
}
