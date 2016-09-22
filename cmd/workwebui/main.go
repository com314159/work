package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/gocraft/work/webui"
	"github.com/FZambia/go-sentinel"
	"errors"
	"strings"
)

var (
	redisNamespace = flag.String("ns", "work", "redis namespace")
	webHostPort    = flag.String("listen", ":5040", "hostport to listen for HTTP JSON API")
	redisUrl = flag.String("redisUrl","","set redis url like redis://:{pwd}@{host:port}/{db}, use this will not use redis sentinal")
	redisSentinals = flag.String("redisSentinals","","set redis sentinals like 120.25.237.253:27000,120.25.237.253:27000")
	redisDatabase = flag.String("database","0","redis database")
	redisPassword = flag.String("redisPwd","","redis password used by redisSentinal, for redis you should set on redis url")
	redisSentinalMaster = flag.String("redisSentinalMaster","","redis sentinal master name")
)

func main() {
	flag.Parse()

	fmt.Println("Starting workwebui:")
	fmt.Println("redis sentinal = ", *redisSentinals)
	fmt.Println("database = ", *redisDatabase)
	fmt.Println("namespace = ", *redisNamespace)
	fmt.Println("listen = ", *webHostPort)

	database, err := strconv.Atoi(*redisDatabase)
	if err != nil {
		fmt.Printf("Error: %v is not a valid database value", *redisDatabase)
		return
	}
	
	var pool *redis.Pool = nil
	
	if *redisUrl != "" {
		pool = newPool(*redisUrl)
	} else {
		
		sentinals := *redisSentinals
		if sentinals == "" {
			fmt.Println("error sentinals is nil")
			return
		}
		
		addrs := strings.Split(sentinals,",")
		
		pool = newSentinelPool(addrs,*redisPassword,database,*redisSentinalMaster)
	}


	server := webui.NewServer(*redisNamespace, pool, *webHostPort)
	server.Start()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)

	<-c

	server.Stop()

	fmt.Println("\nQuitting...")
}

func newPool(reisUrl string) *redis.Pool {
	return &redis.Pool{
		MaxActive:   3,
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			return redis.DialURL(reisUrl)
		},
		Wait: true,
	}
}


func newSentinelPool(addrs []string, pwd string, db int, masterName string) *redis.Pool {
	sntnl := &sentinel.Sentinel{
		Addrs:    addrs,
		MasterName: masterName,
		Dial: func(addr string) (redis.Conn, error) {
			timeout := 500 * time.Millisecond
			c, err := redis.DialTimeout("tcp", addr, timeout, timeout, timeout)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
	}
	return &redis.Pool{
		MaxIdle:     5,
		MaxActive:   5,
		Wait:        true,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			masterAddr, err := sntnl.MasterAddr()
			if err != nil {
				return nil, err
			}
			c, err := redis.Dial("tcp", masterAddr,redis.DialPassword(pwd),redis.DialDatabase(db))
			if err != nil {
				return nil, err
			}
			return c, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if !sentinel.TestRole(c, "master") {
				return errors.New("Role check failed")
			} else {
				return nil
			}
		},
	}
}