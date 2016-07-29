package api

import (
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NetSys/quilt/api/pb"
	"github.com/NetSys/quilt/api/util"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/engine"
	"github.com/NetSys/quilt/stitch"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	log "github.com/Sirupsen/logrus"
)

// DefaultSocket is the socket the Quilt daemon listens on by default.
const DefaultSocket = "unix:///tmp/quilt.sock"

// DefaultRemotePort is the port remote Quilt daemons (the minion) listen on by default.
const DefaultRemotePort = 9000

type server struct {
	dbConn db.Conn
}

// RunServer accepts incoming `quiltctl` connections and responds to them.
func RunServer(conn db.Conn, listenAddr string) error {
	proto, addr, err := util.ParseListenAddress(listenAddr)
	if err != nil {
		return err
	}

	var sock net.Listener
	server := server{conn}
	for {
		sock, err = net.Listen(proto, addr)

		if err != nil {
			log.WithError(err).Error("Failed to open socket.")
		} else {
			break
		}

		time.Sleep(30 * time.Second)
	}

	// Cleanup the socket if we're interrupted.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func(c chan os.Signal) {
		sig := <-c
		log.Printf("Caught signal %s: shutting down.", sig)
		sock.Close()
		os.Exit(0)
	}(sigc)

	s := grpc.NewServer()
	pb.RegisterAPIServer(s, server)
	s.Serve(sock)

	return nil
}

func (s server) QueryMachines(
	cts context.Context, query *pb.DBQuery) (*pb.MachineReply, error) {
	var resp pb.MachineReply
	var machines []db.Machine
	s.dbConn.Transact(func(view db.Database) error {
		machines = view.SelectFromMachine(nil)
		return nil
	})
	for _, m := range machines {
		resp.Machines = append(resp.Machines, &pb.Machine{
			ID:        int32(m.ID),
			Role:      string(m.Role),
			Provider:  string(m.Provider),
			Region:    m.Region,
			Size:      m.Size,
			DiskSize:  int32(m.DiskSize),
			SSHKeys:   m.SSHKeys,
			CloudID:   m.CloudID,
			PublicIP:  m.PublicIP,
			PrivateIP: m.PrivateIP,
			Connected: m.Connected,
		})
	}

	return &resp, nil
}

func (s server) QueryContainers(
	cts context.Context, query *pb.DBQuery) (*pb.ContainerReply, error) {
	var resp pb.ContainerReply
	containers := s.dbConn.SelectFromContainer(nil)
	for _, c := range containers {
		resp.Containers = append(resp.Containers, &pb.Container{
			ID:       int32(c.ID),
			DockerID: c.DockerID,
			Image:    c.Image,
			Command:  c.Command,
			Labels:   c.Labels,
		})
	}
	return &resp, nil
}

func (s server) Run(cts context.Context, runReq *pb.RunRequest) (*pb.RunResult, error) {
	stitch, err := stitch.New(runReq.Stitch)
	if err != nil {
		return &pb.RunResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	err = engine.UpdatePolicy(s.dbConn, stitch)
	if err != nil {
		return &pb.RunResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	return &pb.RunResult{Success: true}, nil
}
