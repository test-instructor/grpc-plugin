package demo

import (
	context "context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/test-instructor/grpc-plugin/demo/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

var CHARS = []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z",
	"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z",
	"1", "2", "3", "4", "5", "6", "7", "8", "9", "0"}

/*RandAllString  生成随机字符串([a~zA~Z0~9])
  lenNum 长度
*/

func RandAllString(lenNum int) string {
	rMutex.Lock()
	defer rMutex.Unlock()
	str := strings.Builder{}
	length := len(CHARS)
	for i := 0; i < lenNum; i++ {
		l := CHARS[r.Intn(length)]
		str.WriteString(l)
	}
	return str.String()
}

type UserServerGRPC struct {
	user.UnimplementedUserServer
}

func (u *UserServerGRPC) UploadImg(ctx context.Context, req *user.UploadImgReq) (*user.UploadImgResp, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		err := errors.New("未登录")
		return nil, err
	}
	token := md.Get("Token")[0]
	id, _ := strconv.Atoi(md.Get("id")[0])
	if UserDB[uint32(id)].Token == token {
		buf := req.Img
		var filePath string
		if req.FileType == user.UploadImgType_JPG {
			filePath = "./src/server/img/" + RandAllString(20) + ".jpg"
		} else if req.FileType == user.UploadImgType_PNG {
			filePath = "./src/server/img/" + RandAllString(20) + ".png"
		} else {
			err := errors.New("不支持的文件类型")
			return nil, err
		}
		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Printf("文件错误,错误为:%v\n", err)
		}
		defer file.Close()
		file.Write(buf)
		userMutex.Lock()
		defer userMutex.Unlock()
		UserDB[uint32(id)].Picture = filePath
		UserDB[uint32(id)].PictureNum++
		resp := user.UploadImgResp{
			Message: filePath,
		}
		return &resp, nil
	} else {
		err := errors.New("未登录")
		return nil, err
	}
	return nil, nil
}

func (u *UserServerGRPC) Cancellation(ctx context.Context, req *user.CancellationReq) (*user.CancellationResp, error) {
	//TODO implement me
	panic("implement me")
}

func (u *UserServerGRPC) GetUserList(ctx context.Context, req *user.GetUserListReq) (*user.GetUserListResp, error) {
	listMutex.RLock()
	defer listMutex.RUnlock()
	if req.Sort == user.UserListSort_ASC {
		sort.Sort(UserList)
	} else {
		sort.Sort(sort.Reverse(UserList))
	}
	uList, _ := json.Marshal(UserList)
	var resp user.GetUserListResp
	err := json.Unmarshal(uList, &resp.UserInfo)
	if err != nil {
		return nil, err
	}
	return &resp, err

}

func (u *UserServerGRPC) UserInfo(ctx context.Context, req *user.UserInfoReq) (*user.UserInfoResp, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		err := errors.New("未登录")
		return nil, err
	}
	token := md.Get("Token")[0]
	id, _ := strconv.Atoi(md.Get("id")[0])
	if UserDB[uint32(id)] == nil {
		err := errors.New("未登录")
		return nil, err
	}
	if UserDB[uint32(id)].Token == token {
		var resp user.UserInfoResp
		resp.ID = UserDB[uint32(id)].ID
		resp.UserName = UserDB[uint32(id)].UserName
		return &resp, nil
	} else {
		err := errors.New("未登录")
		return nil, err
	}
	return nil, nil
}

func (u *UserServerGRPC) Login(ctx context.Context, req *user.LoginReq) (*user.LoginResp, error) {
	fmt.Println("=========================Login")
	md, _ := metadata.FromIncomingContext(ctx)
	fmt.Println(md)

	userName := req.UserName
	userMutex.Lock()
	header := metadata.New(map[string]string{
		"Access-Control-Allow-Headers": "X-Requested-With,content-type,Accept,Authorization",
		"UserName":                     userName,
		"Func":                         "Login",
		//"User":                         user[0],
	})
	grpc.SendHeader(ctx, header)
	defer userMutex.Unlock()
	if len(userName) > 0 {
		_, ok := UserDB[UserDBName[userName]]
		if ok && req.Password == UserDB[UserDBName[userName]].Password {
			var resp user.LoginResp
			UserDB[UserDBName[userName]].Token = RandAllString(32)
			resp.UserName = UserDB[UserDBName[userName]].UserName
			resp.ID = UserDB[UserDBName[userName]].ID
			resp.Token = UserDB[UserDBName[userName]].Token
			return &resp, nil
		}
		err := errors.New("用户名或密码错误")
		return nil, err

	}
	err := errors.New("用户不存在")
	return nil, err
}

func (u *UserServerGRPC) RegisterUser(ctx context.Context, req *user.RegisterUserReq) (*user.RegisterUserResp, error) {
	fmt.Println("=========================RegisterUser")
	md, _ := metadata.FromIncomingContext(ctx)
	fmt.Println(md)
	userID := UserDBName[req.UserName]
	header := metadata.New(map[string]string{"Access-Control-Allow-Headers": "X-Requested-With,content-type,Accept,Authorization", "UserName": req.UserName})
	grpc.SendHeader(ctx, header)
	if userID == 0 {
		uid := UserDB.NexUserID()
		us := &User{}
		us.ID = uid
		us.UserName = req.UserName
		us.Password = req.Password
		us.Sex = req.Sex
		us.RegisterTime = time.Now()
		userMutex.Lock()
		UserDB[uid] = us
		UserDBName[us.UserName] = us.ID
		userMutex.Unlock()

		var resp user.RegisterUserResp
		resp.UserName = req.UserName
		resp.ID = uid

		return &resp, nil
	} else {
		err := errors.New("用户名已存在，请更换用户名")
		return nil, err
	}

}
