package demo

import (
	"github.com/test-instructor/grpc-plugin/demo/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

const (
	port = ":40061"
)

var userMutex sync.RWMutex
var listMutex sync.RWMutex
var r *rand.Rand
var rMutex sync.Mutex
var u *grpc.Server

func StartSvc() {
	r = rand.New(rand.NewSource(time.Now().UnixNano()))
	userMutex = sync.RWMutex{}
	listMutex = sync.RWMutex{}
	UserDB = make(UserMap)
	UserDBName = make(UserMapName)
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	SetTimerTask()
	u = grpc.NewServer()
	user.RegisterUserServer(u, &UserServerGRPC{})
	// Register reflection service on gRPC server.
	reflection.Register(u)
	if err := u.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

}

func StopSvc() {
	u.Stop()
}
