package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	pb "github.com/joyrack/kv3600/protobuf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	FOLLOWER	= "follower"
	LEADER		= "leader"
)

var (
	leaderAddress = flag.String("addr", "localhost:50051", "address of the leader node")
	leaderPort = flag.Int("port", 50051, "The server port")
	role = flag.String("role", FOLLOWER, "the role that this process will play in this cluster")
)

type heartbeatServiceServer struct {
	pb.UnimplementedHeartbeatServiceServer
}

type registrationServiceServer struct {
	pb.UnimplementedRegistrationServiceServer
}

var followerNodes = make([]*pb.Address, 0, 5)

func (s *registrationServiceServer) RegisterFollower(_ context.Context, in *pb.Address) (*pb.RegistrationResponse, error) {
	log.Printf("register follower request received from %s:%s", in.Host, in.Port)
	followerNodes = append(followerNodes, in)
	fmt.Printf("updated followerNodes = %v\n", followerNodes)
	return &pb.RegistrationResponse{Successful: true}, nil
}

func (s *heartbeatServiceServer) SendHeartbeat(_ context.Context, heartbeat *pb.Heartbeat) (*pb.HeartbeatAck, error) {
	log.Printf("Received heartbeat from %d", heartbeat.GetSourceId())
	return &pb.HeartbeatAck{SourceId: int64(os.Getpid())}, nil
}


func main() {
	flag.Parse()
	log.Printf("----- ProcessId: %d -------", os.Getpid())

	if *role == FOLLOWER {
		startProcessAsFollower();
	} else {
		startProcessAsLeader();
	}
}

func startProcessAsFollower() {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterHeartbeatServiceServer(s, &heartbeatServiceServer{})
	log.Printf("follower node listening at %v", lis.Addr())

	if err = registerOnLeader(lis.Addr()); err != nil {
		panic(err)	// terminate?
	}

	// Start follower node	
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func registerOnLeader(addr net.Addr) error {
	conn, err := grpc.NewClient(*leaderAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("unable to connect to leader: %v", err)
	}
	defer conn.Close()
	c := pb.NewRegistrationServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return fmt.Errorf("invalid address. %v", err)
	}
	resp, err := c.RegisterFollower(ctx, &pb.Address{Host: host, Port: port})
	if resp.GetSuccessful() {
		return nil
	} else {
		return fmt.Errorf("could not register on the leader at %s. error: %v", *leaderAddress, err)
	}
}

func startProcessAsLeader() {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *leaderPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterRegistrationServiceServer(s, &registrationServiceServer{})
	log.Printf("leader node listening at %v", lis.Addr())

	// TODO: Should probably keep this in a goroutine since this is not the main purpose of the leader node

	if err = s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

}