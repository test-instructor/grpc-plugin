package demo

import (
	"github.com/robfig/cron/v3"
	"github.com/test-instructor/grpc-plugin/demo/user"
	"sort"
	"sync"
	"time"
)

type timer struct {
	taskList map[string]*cron.Cron
	sync.Mutex
}

func (t *timer) AddTaskByFunc(taskName string, task func()) (cron.EntryID, error) {
	t.Lock()
	defer t.Unlock()
	if _, ok := t.taskList[taskName]; !ok {
		t.taskList[taskName] = cron.New(cron.WithSeconds())
	}
	id, err := t.taskList[taskName].AddFunc("*/10 * * * * *", task)
	t.taskList[taskName].Start()
	return id, err
}

func SetUserW() {
	userMutex.Lock()
	listLen := uint32(len(UserList))
	defer userMutex.Unlock()

	for i := listLen + 1; i < UserID; i++ {
		ue := UserDB[uint32(i)]
		if ue != nil {
			us := UserSimple{
				ID:           ue.ID,
				UserName:     ue.UserName,
				Sex:          ue.Sex,
				RegisterTime: ue.RegisterTime,
			}
			if us.Sex == user.UserSex_Female {
				us.G = 200
			} else {
				us.G = 100
			}

			if ue.PictureNum >= 2 {
				us.A = 100
			} else {
				us.A = 1
			}
			s := time.Since(us.RegisterTime).Seconds()
			if s < 30 {
				us.T = 100
			} else {
				us.T = 3
			}
			us.W = us.A + us.G + us.T
			UserList = append(UserList, &us)
		} else {
			break
		}
	}

	for _, v := range UserList {
		if UserDB[v.ID].PictureNum >= 2 {
			v.A = 100
		} else {
			v.A = 1
		}
		s := time.Since(v.RegisterTime).Seconds()
		if s < 30 {
			v.T = 100
		} else {
			v.T = 3
		}
		v.W = v.A + v.G + v.T

	}
	sort.Sort(UserList)
	//u, _ := json.Marshal(UserList)
	//fmt.Println(string(u))
}

func SetTimerTask() {
	t := timer{taskList: make(map[string]*cron.Cron)}
	t.AddTaskByFunc("排序", SetUserW)
}
