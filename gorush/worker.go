package gorush

import (
	"context"
	"errors"
	"github.com/appleboy/gorush/blacklist"
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
		if err, bl := TokenBlackList.IsInBlacklist(token); err != nil || !bl {
			allowTokens = append(allowTokens, token)
		} else {
			LogError.Warning("Detect blacklist token ", token)
		}
	}

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
		switch notification.Platform {
		case PlatFormIos:
			if !PushConf.Ios.Enabled {
				continue
			}
		case PlatFormAndroid:
			if !PushConf.Android.Enabled {
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
