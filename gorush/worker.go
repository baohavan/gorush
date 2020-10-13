package gorush

import (
	"context"
	"errors"
	"github.com/appleboy/gorush/blacklist"
	"github.com/appleboy/gorush/config"
	"sync"
)

// InitWorkers for initialize all workers.
func InitWorkers(ctx context.Context, wg *sync.WaitGroup, workerNum int64, queueNum int64) {
	LogAccess.Info("worker number is ", workerNum, ", queue number is ", queueNum)
	TokenBlackList = blacklist.NewBlackList(PushConf)
	err := TokenBlackList.Init()
	if err != nil {
		LogAccess.Infof("Init blacklist failed %v", err)
	} else {
		LogAccess.Info("init blacklist success")
	}

	QueueNotification = make(chan PushNotification, queueNum)
	for i := int64(0); i < workerNum; i++ {
		go startWorker(ctx, wg, i)
	}
}

// SendNotification is send message to iOS or Android
func SendNotification(req PushNotification) {
	if PushConf.Core.Sync {
		defer req.WaitDone()
	}

	var allowTokens []string
	for _, token := range req.Tokens {
		if err, dev := TokenBlackList.IsInDevToken(token); err == nil && dev {
			allowTokens = append(allowTokens, token)
			req.Development = true
			LogError.Warningf("Detect dev token %s => Force development", token)
			continue
		}

		if err, bl := TokenBlackList.IsInBlacklist(token); err != nil || !bl {
			allowTokens = append(allowTokens, token)
		} else {
			LogError.Warning("Detect blacklist token ", token)
		}
	}

	LogAccess.Infof("Send notification for app %d", req.AppID)
	req.Tokens = allowTokens

	select {
	case <-req.Ctx.Done():
	default:
		switch req.Platform {
		case PlatFormIos:
			PushToIOS(req)
		case PlatFormAndroid:
			PushToAndroid(req)
		}
	}

	for _, token := range req.BadTokens {
		LogError.Warning("Blacklist token ", token)
		TokenBlackList.Blacklist(token)
	}
}

func startWorker(ctx context.Context, wg *sync.WaitGroup, num int64) {
	defer wg.Done()
	for notification := range QueueNotification {
		SendNotification(notification)
	}
	LogAccess.Info("closed the worker num ", num)
}

// markFailedNotification adds failure logs for all tokens in push notification
func markFailedNotification(notification *PushNotification, reason string) {
	LogError.Error(reason)
	for _, token := range notification.Tokens {
		notification.AddLog(getLogPushEntry(FailedPush, token, *notification, errors.New(reason)))
	}
	notification.WaitDone()
}

// queueNotification add notification to queue list.
func queueNotification(ctx context.Context, req RequestPush) (int, []LogPushEntry) {
	var count int
	wg := sync.WaitGroup{}
	newNotification := []*PushNotification{}
	for i := range req.Notifications {
		notification := &req.Notifications[i]
		var enableAndroid, enableIos bool
		if notification.AppID == 0 {
			enableAndroid = PushConf.Android.Enabled
			enableIos = PushConf.Ios.Enabled
		} else {
			conf := config.GetApplicationConfig(&PushConf, notification.AppID)
			if conf == nil {
				LogError.Warningf("Missing config for application %d => Skip queuing", notification.AppID)
				continue
			}

			enableAndroid = conf.Android.Enabled
			enableIos = conf.Ios.Enabled
		}

		switch notification.Platform {
		case PlatFormIos:
			if !enableAndroid {
				continue
			}
		case PlatFormAndroid:
			if !enableIos {
				continue
			}
		}
		newNotification = append(newNotification, notification)
	}

	log := make([]LogPushEntry, 0, count)
	for _, notification := range newNotification {
		notification.Ctx = ctx
		if PushConf.Core.Sync {
			notification.wg = &wg
			notification.log = &log
			notification.AddWaitCount()
		}
		if !tryEnqueue(*notification, QueueNotification) {
			markFailedNotification(notification, "max capacity reached")
		}
		count += len(notification.Tokens)
		// Count topic message
		if notification.To != "" {
			count++
		}
	}

	if PushConf.Core.Sync {
		wg.Wait()
	}

	StatStorage.AddTotalCount(int64(count))

	return count, log
}

// tryEnqueue tries to enqueue a job to the given job channel. Returns true if
// the operation was successful, and false if enqueuing would not have been
// possible without blocking. Job is not enqueued in the latter case.
func tryEnqueue(job PushNotification, jobChan chan<- PushNotification) bool {
	select {
	case jobChan <- job:
		return true
	default:
		return false
	}
}
