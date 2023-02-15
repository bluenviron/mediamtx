package core

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/redis/go-redis/v9"
	"log"
)

type redisServerParent interface {
	Log(logger.Level, string, ...interface{})
}
type RedisInstance struct {
	rdb       *redis.Client
	ctx       context.Context
	EventName string
	parent    redisServerParent
}

func NewRedis(ctx context.Context, Address string, Port string, Username string, Password string, Database int, EventName string, parent redisServerParent) *RedisInstance {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", Address, Port),
		Password: Password, // no password set
		Username: Username, // no password set
		DB:       Database, // use default DB
	})
	redisInstance := &RedisInstance{ctx: ctx, rdb: rdb, EventName: EventName, parent: parent}

	redisInstance.log(logger.Info, "Redis Initialized (%s:%s)", Address, Port)

	return redisInstance
}

func (receiver *RedisInstance) Close() {
	err := receiver.rdb.Close()
	if err != nil {
		return
	}

}

func (receiver *RedisInstance) Publish(event EventStream) {
	jsonData, jsonError := json.Marshal(event)
	if jsonError != nil {
		log.Fatal(jsonError)
	}
	receiver.rdb.Publish(receiver.ctx, receiver.EventName, jsonData)
}

func (receiver *RedisInstance) log(level logger.Level, format string, args ...interface{}) {
	label := "REDIS"
	receiver.parent.Log(level, "[%s] "+format, append([]interface{}{label}, args...)...)
}
