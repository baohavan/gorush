package config

type SectionApplication struct {
	Android SectionAndroid `yaml:"android"`
	Ios     SectionIos     `yaml:"ios"`
	AppID   int            `yaml:"app_id"`
}
