package blacklist

import (
	"fmt"
	"github.com/appleboy/gorush/config"
	"github.com/cespare/xxhash/v2"
	"gopkg.in/redis.v5"
	"log"
	"time"
)

type BlackList struct {
	config      config.ConfYaml
	redisClient *redis.Client
}

func NewBlackList(config config.ConfYaml) *BlackList {
	return &BlackList{
		config: config,
	}
}

func (b *BlackList) Init() error {
	log.Printf("Blacklist config %v", b.config.BlackList.Redis)
	b.redisClient = redis.NewClient(&redis.Options{
		Addr:     b.config.BlackList.Redis.Addr,
		Password: b.config.BlackList.Redis.Password,
		DB:       b.config.BlackList.Redis.DB,
	})

	_, err := b.redisClient.Ping().Result()

	if err != nil {
		// redis server error
		log.Println("Can't connect redis server: " + err.Error())

		return err
	}

	return nil
}

func (b *BlackList) Blacklist(token string) error {
	_, err := b.redisClient.Set(fmt.Sprintf("bl:%d", xxhash.Sum64String(token)), token, time.Hour*24).Result()
	return err
}

func (b *BlackList) IsInBlacklist(token string) (error, bool) {
	res, err := b.redisClient.Get(fmt.Sprintf("bl:%d", xxhash.Sum64String(token))).Result()
	return err, res != ""
}
