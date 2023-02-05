package demo

import (
	"github.com/test-instructor/grpc-plugin/demo/user"
	"time"
)

var UserID uint32

type User struct {
	ID           uint32
	UserName     string
	P            string
	NickName     string
	Sex          user.UserSex
	RegisterTime time.Time
	Picture      string
	PictureNum   int
	Token        string
}

type UserMap map[uint32]*User
type UserMapName map[string]uint32

func (u *UserMap) NexUserID() uint32 {
	UserID++
	return UserID
}

type UserSimple struct {
	ID           uint32
	UserName     string
	Sex          user.UserSex
	RegisterTime time.Time
	G            int
	T            int
	A            int
	W            int
}

type Users []*UserSimple

type UserSortW Users

func (u UserSortW) Len() int {
	return len(u)
}

func (u UserSortW) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

func (u UserSortW) Less(i, j int) bool {
	return u[i].W < u[j].W
}

var UserDB UserMap
var UserDBName UserMapName
var UserList UserSortW
