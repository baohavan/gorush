package gorush

import (
	"errors"
	"fmt"
	"github.com/appleboy/gorush/config"
	"github.com/baohavan/go-fcm"
	"github.com/sirupsen/logrus"
	"strings"
)

// InitFCMClient use for initialize FCM Client for specific app.
func InitFCMClientByAppID(appID int) error {
	var err error

	conf := config.GetApplicationConfig(&PushConf, appID)
	if conf == nil {
		return errors.New(fmt.Sprintf("Not found app config %d", appID))
	}

	if conf.Android.Credentials == "" {
		err = InitFCMClientByApiKeyByAppID(appID, conf.Android)
	} else {
		err = InitFCMClientByCredentialsByAppId(appID, conf.Android)
	}

	return err
}

// InitFCMClient use for initialize FCM Client.
func InitFCMClient() (*fcm.Client, error) {
	var err error

	if PushConf.Android.Credentials == "" {
		FCMClient, err = InitFCMClientByApiKey(PushConf.Android.APIKey)
	} else {
		FCMClient, err = InitFCMClientByCredentials(PushConf.Android.Credentials)
	}

	return FCMClient, err
}

// InitFCMClientByApiKey use for initialize FCM Client.
func InitFCMClientByApiKey(key string) (*fcm.Client, error) {
	var err error

	if key == "" {
		return nil, errors.New("Missing Android API Key")
	}

	if key != PushConf.Android.APIKey {
		return fcm.NewClient(key)
	}

	if FCMClient == nil {
		LogAccess.Debug("Init NewClient client")
		FCMClient, err = fcm.NewClient(key)
		return FCMClient, err
	}

	return FCMClient, nil
}

// InitFCMClientByApiKey use for initialize FCM Client.
func InitFCMClientByApiKeyByAppID(appID int, conf config.SectionAndroid) error {
	var err error

	if conf.APIKey == "" {
		return errors.New("Missing Android API Key")
	}

	if ApplicationList[appID].FCMClient == nil {
		LogAccess.Infof("Init NewClient client for app %d with key %s", appID, conf.APIKey)
		ApplicationList[appID].FCMClient, err = fcm.NewClient(conf.APIKey)
		return err
	}

	return nil
}

// InitFCMClient use for initialize FCM Client.
func InitFCMClientByCredentials(credentials string) (*fcm.Client, error) {
	var err error

	if credentials == "" {
		return nil, errors.New("Missing Android API Key")
	}

	if credentials != PushConf.Android.Credentials {
		return fcm.NewClientWithCredentials(credentials)
	}

	if FCMClient == nil {
		LogAccess.Infof("Init NewClientWithCredentials client %s", credentials)
		FCMClient, err = fcm.NewClientWithCredentials(credentials)
		return FCMClient, err
	}

	return FCMClient, nil
}

// InitFCMClient use for initialize FCM Client.
func InitFCMClientByCredentialsByAppId(appID int, conf config.SectionAndroid) error {
	var err error

	credentials := conf.Credentials
	if credentials == "" {
		return errors.New("Missing Android Credentials")
	}

	if ApplicationList[appID].FCMClient == nil {
		LogAccess.Debugf("Init NewClientWithCredentials client %s", credentials)
		ApplicationList[appID].FCMClient, err = fcm.NewClientWithCredentials(credentials)
		return err
	}

	return nil
}

// GetAndroidNotification use for define Android notification.
// HTTP Connection Server Reference for Android
// https://firebase.google.com/docs/cloud-messaging/http-server-ref
func GetAndroidNotification(req PushNotification) *fcm.Message {
	notification := &fcm.Message{
		To:                    req.To,
		Condition:             req.Condition,
		CollapseKey:           req.CollapseKey,
		ContentAvailable:      req.ContentAvailable,
		MutableContent:        req.MutableContent,
		DelayWhileIdle:        req.DelayWhileIdle,
		TimeToLive:            req.TimeToLive,
		RestrictedPackageName: req.RestrictedPackageName,
		DryRun:                req.DryRun,
	}

	if len(req.Tokens) > 0 {
		notification.RegistrationIDs = req.Tokens
	}

	if len(req.Priority) > 0 && req.Priority == "high" {
		notification.Priority = "high"
	}

	// Add another field
	if len(req.Data) > 0 {
		notification.Data = make(map[string]interface{})
		for k, v := range req.Data {
			notification.Data[k] = v
		}
	}

	n := &fcm.Notification{}
	isNotificationSet := false
	if req.Notification != nil {
		isNotificationSet = true
		n = req.Notification
	}

	if len(req.Message) > 0 {
		isNotificationSet = true
		n.Body = req.Message
	}

	if len(req.Title) > 0 {
		isNotificationSet = true
		n.Title = req.Title
	}

	if len(req.Image) > 0 {
		isNotificationSet = true
		n.Image = req.Image
	}

	if v, ok := req.Sound.(string); ok && len(v) > 0 {
		isNotificationSet = true
		n.Sound = v
	}

	if isNotificationSet {
		notification.Notification = n
	}

	// handle iOS apns in fcm

	if len(req.Apns) > 0 {
		notification.Apns = req.Apns
	}

	return notification
}

func getFcmClient(req PushNotification) (client *fcm.Client, err error) {
	if req.APIKey != "" {
		client, err = InitFCMClientByApiKey(req.APIKey)
	} else {
		if req.AppID == 0 {
			client, err = InitFCMClient()
		} else {
			client = ApplicationList[req.AppID].FCMClient
			if client == nil {
				err = errors.New(fmt.Sprintf("Fcm client for app %d not config", req.AppID))
			}
		}
	}
	return
}

// PushToAndroid provide send notification to Android server.
func PushToAndroid(req PushNotification) bool {
	LogAccess.Debug("Start push notification for Android")

	var (
		client     *fcm.Client
		retryCount = 0
		maxRetry   = PushConf.Android.MaxRetry
	)

	if req.Retry > 0 && req.Retry < maxRetry {
		maxRetry = req.Retry
	}

	// check message
	err := CheckMessage(req)

	if err != nil {
		LogError.Error("request error: " + err.Error())
		return false
	}

Retry:
	var isError = false

	notification := GetAndroidNotification(req)

	//if req.APIKey != "" {
	//	client, err = InitFCMClientByApiKey(req.APIKey)
	//} else {
	//	if req.AppID == 0 {
	//		client, err = InitFCMClient()
	//	} else {
	//		client = ApplicationList[req.AppID].FCMClient
	//	}
	//}
	client, err = getFcmClient(req)

	if err != nil {
		// FCM server error
		LogError.Error("FCM server error: " + err.Error())
		return false
	}

	res, err := client.Send(notification)
	if err != nil {
		// Send Message error
		LogError.Error("FCM server send message error: " + err.Error())
		return false
	}

	if !req.IsTopic() {
		LogAccess.Debug(fmt.Sprintf("Android Success count: %d, Failure count: %d", res.Success, res.Failure))
	}

	StatStorage.AddAndroidSuccess(int64(res.Success))
	StatStorage.AddAndroidError(int64(res.Failure))

	var newTokens []string
	// result from Send messages to specific devices
	for k, result := range res.Results {
		to := ""
		if k < len(req.Tokens) {
			to = req.Tokens[k]
		} else {
			to = req.To
		}

		if result.Error != nil {
			isError = true
			newTokens = append(newTokens, to)
			LogPush(FailedPush, to, req, result.Error)
			//unregistered device for android
			if strings.Contains(result.Error.Error(), "unregistered device") {
				//req.BadTokens = append(req.BadTokens, to)
				LogError.Warning("Detect unregistered device token ", to)
				if err = TokenBlackList.Blacklist(to); err != nil {
					LogError.Warning("Blacklist unregistered device token failed ", err)
				} else {
					LogError.Info("Blacklist unregistered device toke success")
				}
			}

			if PushConf.Core.Sync {
				req.AddLog(getLogPushEntry(FailedPush, to, req, result.Error))
			} else if PushConf.Core.FeedbackURL != "" {
				go func(logger *logrus.Logger, log LogPushEntry, url string, timeout int64) {
					err := DispatchFeedback(log, url, timeout)
					if err != nil {
						logger.Error(err)
					}
				}(LogError, getLogPushEntry(FailedPush, to, req, result.Error), PushConf.Core.FeedbackURL, PushConf.Core.FeedbackTimeout)
			}
			continue
		}

		LogPush(SucceededPush, to, req, nil)
	}

	// result from Send messages to topics
	if req.IsTopic() {
		to := ""
		if req.To != "" {
			to = req.To
		} else {
			to = req.Condition
		}
		LogAccess.Debug("Send Topic Message: ", to)
		// Success
		if res.MessageID != 0 {
			LogPush(SucceededPush, to, req, nil)
		} else {
			isError = true
			// failure
			LogPush(FailedPush, to, req, res.Error)
			if PushConf.Core.Sync {
				req.AddLog(getLogPushEntry(FailedPush, to, req, res.Error))
			}
		}
	}

	// Device Group HTTP Response
	if len(res.FailedRegistrationIDs) > 0 {
		isError = true
		newTokens = append(newTokens, res.FailedRegistrationIDs...)

		LogPush(FailedPush, notification.To, req, errors.New("device group: partial success or all fails"))
		if PushConf.Core.Sync {
			req.AddLog(getLogPushEntry(FailedPush, notification.To, req, errors.New("device group: partial success or all fails")))
		}
	}

	if isError && retryCount < maxRetry {
		retryCount++

		// resend fail token
		req.Tokens = newTokens

		isError = false
		goto Retry
	}

	return isError
}
