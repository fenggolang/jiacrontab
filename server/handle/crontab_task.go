package handle

import (
	"crypto/md5"
	"fmt"
	"jiacrontab/libs"
	"jiacrontab/libs/proto"
	"jiacrontab/libs/rpc"
	"jiacrontab/model"
	"jiacrontab/server/conf"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/kataras/iris"
)

// var app *jiaweb.JiaWeb

const (
	minuteTimeLayout = "200601021504"
	dateTimeLayout   = "2006-01-02 15:04:05"
)

func ListTask(ctx iris.Context) {

	var systemInfo map[string]interface{}
	var locals []model.CrontabTask
	var clientList []model.Client
	var client model.Client
	var r = ctx.Request()

	addr := ctx.FormValue("addr")
	if strings.TrimSpace(addr) == "" {
		ctx.ViewData("error", "参数错误")
		ctx.View("public/error.html")
		return
	}

	model.DB().Model(&model.Client{}).Find(&clientList)

	if len(clientList) == 0 {
		if ctx.IsAjax() {
			ctx.JSON(map[string]interface{}{
				"code": -1,
			})
			return
		}

		ctx.ViewData("error", "nothing to show")
		ctx.View("public/error.html")
		return
	}

	for _, v := range clientList {
		if v.Addr == addr {
			client = v
			break
		}
	}

	if err := rpcCall(addr, "CrontabTask.All", "", &locals); err != nil {

		if ctx.IsAjax() {
			ctx.JSON(map[string]interface{}{
				"code": -1,
			})
			return
		}
		log.Println(err)
		ctx.Redirect("/", http.StatusFound)
		return
	}

	if err := rpcCall(addr, "Admin.SystemInfo", "", &systemInfo); err != nil {
		if ctx.IsAjax() {
			ctx.JSON(map[string]interface{}{
				"code": -1,
			})
			return
		}
		ctx.Redirect("/", http.StatusFound)
		return
	}

	if ctx.IsAjax() {
		ctx.JSON(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"taskList":   locals,
				"clientList": clientList,
				"systemInfo": systemInfo,
				"url":        r.RequestURI,
			},
		})
		return
	}

	ctx.ViewData("tasklist", locals)
	ctx.ViewData("addr", addr)
	ctx.ViewData("clientList", clientList)
	ctx.ViewData("client", client)
	ctx.ViewData("systemInfo", systemInfo)
	ctx.ViewData("url", r.RequestURI)
	ctx.View("crontab/list.html")

}

// Index 服务器列表页面
func Index(ctx iris.Context) {
	sInfo := libs.SystemInfo(conf.AppService.ServerStartTime)

	var clientList []model.Client
	model.DB().Model(&model.Client{}).Find(&clientList)

	for k, v := range clientList {
		if time.Now().Sub(v.UpdatedAt) > 10*time.Minute {
			clientList[k].State = 0
		}
	}

	ctx.ViewData("clientList", clientList)
	ctx.ViewData("systemInfoList", sInfo)
	ctx.View("index.html")

}

func EditTask(ctx iris.Context) {
	var reply bool
	var r = ctx.Request()

	sortedKeys := make([]string, 0)
	addr := ctx.FormValue("addr")
	id := uint(libs.ParseInt(ctx.FormValue("taskId")))
	if addr == "" {
		ctx.ViewData("error", "params error")
		ctx.View("public/error.html")
		return
	}

	if r.Method == http.MethodPost {
		var unExitM, unExitA, sync bool
		var pipeCommandList [][]string
		var command string
		var args string

		n := ctx.PostValueTrim("taskName")
		timeoutStr := ctx.PostValueTrim("timeout")
		mConcurrentStr := ctx.PostValueTrim("maxConcurrent")
		unpdExitM := r.FormValue("unexpectedExitMail")
		unpdExitA := r.FormValue("unexpectedExitApi")
		mSync := r.FormValue("sync")
		mailTo := ctx.PostValueTrim("mailTo")
		apiTo := ctx.PostValueTrim("apiTo")
		optimeout := ctx.PostValueTrim("optimeout")
		pipeCommands := r.PostForm["command"]
		pipeArgs := r.PostForm["args"]
		destSli := r.PostForm["depends[dest]"]
		cmdSli := r.PostForm["depends[command]"]
		argsSli := r.PostForm["depends[args]"]
		timeoutSli := r.PostForm["depends[timeout]"]
		depends := make(model.DependsTasks, len(destSli))

		for k, v := range pipeCommands {
			if k == 0 {
				command = v
				args = pipeArgs[0]
			} else {
				pipeCommandList = append(pipeCommandList, []string{v, pipeArgs[k]})
			}

		}

		for k, v := range destSli {
			depends[k].Dest = v
			depends[k].From = addr
			depends[k].Args = argsSli[k]
			tmpT, err := strconv.Atoi(timeoutSli[k])

			if err != nil {
				depends[k].Timeout = 0
			} else {
				depends[k].Timeout = int64(tmpT)
			}
			depends[k].Command = cmdSli[k]
		}

		if unpdExitM == "true" {
			unExitM = true
		}

		if unpdExitA == "true" {
			unExitA = true
		}

		if mSync == "true" {
			sync = true
		}

		if _, ok := map[string]bool{"email": true, "api": true, "kill": true, "email_and_kill": true, "ignore": true}[optimeout]; !ok {
			optimeout = "ignore"
		}
		timeout, err := strconv.Atoi(timeoutStr)
		if err != nil {
			timeout = 0
		}

		maxConcurrent, err := strconv.Atoi(mConcurrentStr)
		if err != nil {
			maxConcurrent = 10
		}

		month := libs.ReplaceEmpty(strings.TrimSpace(r.FormValue("month")), "*")
		weekday := libs.ReplaceEmpty(strings.TrimSpace(r.FormValue("weekday")), "*")
		day := libs.ReplaceEmpty(strings.TrimSpace(r.FormValue("day")), "*")
		hour := libs.ReplaceEmpty(strings.TrimSpace(r.FormValue("hour")), "*")
		minute := libs.ReplaceEmpty(strings.TrimSpace(r.FormValue("minute")), "*")

		rpcArgs := model.CrontabTask{
			Name:               n,
			Command:            command,
			Args:               args,
			PipeCommands:       pipeCommandList,
			Timeout:            int64(timeout),
			OpTimeout:          optimeout,
			Create:             time.Now().Unix(),
			MailTo:             mailTo,
			ApiTo:              apiTo,
			MaxConcurrent:      maxConcurrent,
			Depends:            depends,
			UnexpectedExitMail: unExitM,
			UnexpectedExitApi:  unExitA,
			Sync:               sync,
			C: struct {
				Weekday string
				Month   string
				Day     string
				Hour    string
				Minute  string
			}{

				Month:   month,
				Day:     day,
				Hour:    hour,
				Minute:  minute,
				Weekday: weekday,
			},
		}
		rpcArgs.ID = id

		if err := rpcCall(addr, "CrontabTask.Update", rpcArgs, &reply); err != nil {
			ctx.ViewData("error", err.Error())
			ctx.View("public/error.html")
			return
		}

		ctx.Redirect("/crontab/task/list?addr="+addr, http.StatusFound)
		return

	} else {
		var t model.CrontabTask
		var clientList []model.Client

		if id != 0 {
			err := rpcCall(addr, "CrontabTask.Get", id, &t)
			if err != nil {
				ctx.Redirect("/crontab/task/list?addr="+addr, http.StatusFound)
				return

			}
		} else {
			var client model.Client
			model.DB().Find(&client, "addr", addr)
			// client, _ := m.SearchRPCClientList(addr)
			t.MailTo = client.Mail
		}
		if t.MaxConcurrent == 0 {
			t.MaxConcurrent = 1
		}

		model.DB().Find(&clientList)

		if len(clientList) == 0 {
			ctx.ViewData("error", "nothing to show")
			ctx.View("public/error.html")
			return
		}

		ctx.ViewData("addr", addr)
		ctx.ViewData("addrs", sortedKeys)
		ctx.ViewData("clientList", clientList)
		ctx.ViewData("task", t)
		ctx.ViewData("allowCommands", conf.AppService.AllowCommands)
		ctx.View("crontab/edit.html")
	}

}

func StopTask(ctx iris.Context) {
	var r = ctx.Request()

	taskId := ctx.FormValue("taskId")
	addr := ctx.FormValue("addr")
	action := libs.ReplaceEmpty(r.FormValue("action"), "stop")
	var reply bool
	if taskId == "" || addr == "" {
		ctx.ViewData("error", "参数错误")
		ctx.View("public/error.html")
		return
	}

	var method string
	if action == "stop" {
		method = "CrontabTask.Stop"
	} else if action == "delete" {
		method = "CrontabTask.Delete"
	} else {
		method = "CrontabTask.Kill"
	}
	if err := rpcCall(addr, method, taskId, &reply); err != nil {
		ctx.ViewData("error", err)
		ctx.View("public/error.html")
		return
	}
	if reply {
		ctx.Redirect("/crontab/task/list?addr="+addr, http.StatusFound)
		return
	}

	ctx.ViewData("error", fmt.Sprintf("failed %s %s", method, taskId))
	ctx.View("public/error.html")
}

func StopAllTask(ctx iris.Context) {
	var r = ctx.Request()

	taskIds := strings.TrimSpace(r.FormValue("taskIds"))
	addr := strings.TrimSpace(r.FormValue("addr"))
	method := "CrontabTask.StopAll"
	taskIdSli := strings.Split(taskIds, ",")
	var reply bool
	if len(taskIdSli) == 0 || addr == "" {
		ctx.ViewData("error", "参数错误")
		ctx.View("public/error.html")
		return
	}

	if err := rpcCall(addr, method, taskIdSli, &reply); err != nil {
		ctx.ViewData("error", err)
		ctx.View("public/error.html")
		return
	}
	if reply {
		ctx.Redirect("/crontab/task/list?addr="+addr, http.StatusFound)
		return
	}

	ctx.ViewData("error", fmt.Sprintf("failed %s %v", method, taskIdSli))
	ctx.View("public/error.html")
}

func StartTask(ctx iris.Context) {
	var r = ctx.Request()

	taskId := strings.TrimSpace(r.FormValue("taskId"))
	addr := strings.TrimSpace(r.FormValue("addr"))
	var reply bool
	if taskId == "" || addr == "" {
		ctx.ViewData("error", "参数错误")
		ctx.View("public/error.html")
		return
	}

	if err := rpcCall(addr, "CrontabTask.Start", taskId, &reply); err != nil {
		ctx.ViewData("error", "参数错误")
		ctx.View("public/error.html")
		return
	}

	if reply {
		ctx.Redirect("/crontab/task/list?addr="+addr, http.StatusFound)
		return
	}

	ctx.ViewData("error", "failed start task"+taskId)
	ctx.View("public/error.html")
}

func Login(ctx iris.Context) {
	var r = ctx.Request()
	if r.Method == http.MethodPost {

		u := r.FormValue("username")
		pwd := r.FormValue("passwd")
		remb := r.FormValue("remember")

		if u == conf.AppService.User && pwd == conf.AppService.Passwd {

			clientFeature := ctx.RemoteAddr() + "-" + ctx.Request().Header.Get("User-Agent")

			clientSign := fmt.Sprintf("%x", md5.Sum([]byte(clientFeature)))
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user":       u,
				"clientSign": clientSign,
			}).SignedString([]byte(conf.JwtService.JWTSigningKey))

			if err != nil {
				ctx.ViewData("error", fmt.Sprint("无法生成访问凭证:", err))
				ctx.View("public/error.html")
				return
			}
			if remb == "on" {
				ctx.SetCookieKV(conf.JwtService.TokenCookieName, url.QueryEscape(token), iris.CookiePath("/"),
					iris.CookieExpires(time.Duration(conf.JwtService.TokenExpires)*time.Second),
					iris.CookieHTTPOnly(true))

			} else {
				ctx.SetCookieKV(conf.JwtService.TokenCookieName, url.QueryEscape(token))
			}

			ctx.Redirect("/", http.StatusFound)
			return
		}

		ctx.ViewData("error", "auth failed")
		ctx.View("public/error.html")
		return
	}

	_, ok := ctx.Values().Get("jwt").(*jwt.Token)
	if ok {
		ctx.Redirect("/", http.StatusFound)
	}
	ctx.View("login.html")

}

func QuickStart(ctx iris.Context) {
	var r = ctx.Request()

	taskId := strings.TrimSpace(r.FormValue("taskId"))
	addr := strings.TrimSpace(r.FormValue("addr"))
	var reply []byte
	if taskId == "" || addr == "" {
		ctx.ViewData("error", "参数错误")
		ctx.View("public/error.html")
		return
	}

	if err := rpcCall(addr, "CrontabTask.QuickStart", taskId, &reply); err != nil {
		ctx.ViewData("error", err.Error())
		ctx.View("public/error.html")
		return

	}
	logList := strings.Split(string(reply), "\n")

	ctx.ViewData("logList", logList)
	ctx.View("crontab/log.html")
}

func Logout(ctx iris.Context) {
	ctx.RemoveCookie(conf.JwtService.TokenCookieName)
	ctx.Redirect("/login", http.StatusFound)

}

func RecentLog(ctx iris.Context) {
	var r = ctx.Request()
	var searchRet proto.SearchLogResult
	addr := r.FormValue("addr")
	pagesize := 50
	id, err := strconv.Atoi(r.FormValue("taskId"))

	if err != nil {
		ctx.ViewData("error", "参数错误")
		ctx.View("public/error.html")
		return
	}

	page, err := strconv.Atoi(r.FormValue("page"))
	if err != nil || page == 0 {
		page = 1
	}

	date := r.FormValue("date")
	pattern := r.FormValue("pattern")
	isTail := true
	if r.FormValue("isTail") == "false" {
		isTail = false
	}

	if err := rpcCall(addr, "CrontabTask.Log", proto.SearchLog{
		TaskId:   id,
		Page:     page,
		Pagesize: pagesize,
		Date:     date,
		Pattern:  pattern,
		IsTail:   isTail,
	}, &searchRet); err != nil {
		ctx.ViewData("error", err)
	}
	logList := strings.Split(string(searchRet.Content), "\n")

	ctx.ViewData("logList", logList)
	ctx.ViewData("addr", addr)
	ctx.ViewData("total", searchRet.Total)
	ctx.ViewData("pagesize", pagesize)
	ctx.View("crontab/log.html")

}

func Readme(ctx iris.Context) {
	ctx.View("readme.html")
}

func DeleteClient(ctx iris.Context) {

	r := ctx.Request()
	addr := r.FormValue("addr")
	model.DB().Unscoped().Delete(&model.Client{}, "addr=?", addr)
	rpc.Del(addr)
	ctx.Redirect("/", http.StatusFound)
}
