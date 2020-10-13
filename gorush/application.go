package gorush

func InitApplication() {
	ApplicationList = make(map[int]*ApplicationClient)
	for _, app := range PushConf.AppConfigs.Apps {
		LogAccess.Infof("Init config for app %d", app.AppID)
		ApplicationList[app.AppID] = &ApplicationClient{}
		if app.Android.Enabled {
			LogAccess.Infof("Init Android config for app %d", app.AppID)
			InitFCMClientByAppID(app.AppID)
		}
		if app.Ios.Enabled {
			LogAccess.Infof("Init iOS config for app %d", app.AppID)
			InitAPNSClientByAppID(app.AppID)
		}

		LogAccess.Infof("Init config for app %d done", app.AppID)
	}
}
