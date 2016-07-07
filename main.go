package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"syscall"
	"time"

	"github.com/streamrail/concurrent-map"
	log "github.com/unchartedsoftware/plog"
	"github.com/zenazn/goji/graceful"
	"github.com/zenazn/goji/web"

	"github.com/unchartedsoftware/egol/api/conf"
	"github.com/unchartedsoftware/egol/api/middleware"
	"github.com/unchartedsoftware/egol/api/redis"
	"github.com/unchartedsoftware/egol/api/sim"
	"github.com/unchartedsoftware/egol/api/util"
	"github.com/unchartedsoftware/egol/api/ws"
)

const (
	websocketRoute = "/connect"
)

var (
	exit      = make(chan bool)
	organisms map[string]*sim.Organism
	redisConn *redis.Connection
	clients   cmap.ConcurrentMap
	config    *conf.Conf
)

func handleMessage(client *ws.Client) ws.RequestHandler {
	return func(conn *ws.Connection, msg []byte) {
		// parse the tile request
		_, err := ws.NewMessage(msg)
		if err != nil {
			// parsing error, send back a failure response
			err := fmt.Errorf("Unable to parse message: %s", string(msg))
			// log error
			log.Warn(err)
			err = conn.Send(&ws.Message{
				Success: false,
			})
			if err != nil {
				log.Warn(err)
			}
			return
		}
		// send response
		err = conn.Send(&ws.Message{
			Success: true,
		})
		if err != nil {
			log.Warn(err)
		}
	}
}

func shouldExit() bool {
	select {
	case <-exit:
		return true
	default:
		// non blocking
		return false
	}
}

func initializeSim() {
	count := 4
	organisms = make(map[string]*sim.Organism)

	// debug states for 3 organisms
	for i := 0; i < count; i++ {
		id := util.RandID()
		organisms[id] = &sim.Organism{
			ID: id,
			State: &sim.State{
				Type:     "alive",
				Position: sim.RandomPosition(),
			},
			Attributes: &sim.Attributes{
				Family:         uint32(rand.Intn(3)),
				Hunger:         0.0,
				Energy:         1.0,
				Offense:        uint32(rand.Intn(10)),
				Defense:        uint32(rand.Intn(10)),
				Agility:        uint32(rand.Intn(10)),
				Range:          rand.Float64() * 100,
				Reproductivity: uint32(rand.Intn(10)),
				Size:           rand.Float64(),
			},
		}
	}
}

func store(suffix string, iteration int64, data interface{}) error {
	key := fmt.Sprintf("%s-%d-%s", config.SimID, iteration, suffix)
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	err = redisConn.Set(key, bytes)
	if err != nil {
		return err
	}
	return nil
}

func loop() {
	iteration := int64(0)

	for {
		// get timestamp
		stamp := util.Timestamp()

		// check if should exit
		if shouldExit() {
			break
		}

		// apply constraints to each organism
		sim.ApplyConstraints(organisms)

		// determine AI input for each organism
		updates := sim.Iterate(organisms)

		// write out current state
		err := store("state", iteration, organisms)
		if err != nil {
			log.Error(err)
			continue
		}

		// write out updates
		err = store("update", iteration, updates)
		if err != nil {
			log.Error(err)
			continue
		}

		log.Info("Iteration: ", iteration)
		for i := range organisms {
			// log.Info("organisms: ", organisms[i].State)
			log.Info("updates: ", updates[i].State)
		}

		for iter := range clients.Iter() {
			client, ok := iter.Val.(*ws.Client)
			if !ok {
				log.Error(err)
				continue
			}
			// broadcast update to connected clients
			if !client.New {
				err := client.Conn.Send(&ws.Message{
					Type:    "update",
					Data:    updates,
					Success: true,
				})
				if err != nil {
					log.Error(err)
				}
			}
			// broadcast state to new clients
			if client.New {
				err := client.Conn.Send(&ws.Message{
					Type:    "state",
					Data:    organisms,
					Success: true,
				})
				if err != nil {
					log.Error(err)
				}
				client.New = false
			}
		}

		// wait
		now := util.Timestamp()
		elapsed := now - stamp
		if elapsed < config.FrameMS {
			time.Sleep(time.Duration(config.FrameMS-elapsed) * time.Millisecond)
		}

		// increment the iteration
		iteration++
	}
	exit <- true
}

func onWSConnection(w http.ResponseWriter, r *http.Request) {
	log.Info("New connection")
	// create client
	client := ws.NewClient()
	// create connection
	conn, err := ws.NewConnection(w, r, handleMessage(client))
	if err != nil {
		log.Warn(err)
		return
	}
	client.Conn = conn
	clients.Set(client.ID, client)
	// listen for requests and respond
	err = conn.ListenAndRespond()
	if err != nil {
		log.Info(err)
	}
	// clean up internals
	conn.Close()
	clients.Remove(client.ID)
	log.Info("Connection lost")
}

func server() {
	// create server
	app := web.New()
	// mount logger middleware
	app.Use(middleware.Log)
	// mount gzip middleware
	app.Use(middleware.Gzip)
	// add websocket route
	app.Get(websocketRoute, onWSConnection)
	// add greedy route last
	app.Get("/*", http.FileServer(http.Dir(config.PublicDir)))
	// catch kill signals for graceful shutdown
	graceful.AddSignal(syscall.SIGINT, syscall.SIGTERM)
	// start server
	err := graceful.ListenAndServe(":"+config.Port, app)
	if err != nil {
		log.Error(err)
	}
	// exit loop
	exit <- true
	// wait for acknowledgement
	<-exit
}

func main() {

	rand.Seed(time.Now().UnixNano())

	// sets the maximum number of CPUs that can be executing simultaneously
	runtime.GOMAXPROCS(runtime.NumCPU())

	// parse flags into config struct
	config = conf.ParseCommandLine()

	// initialize the sim
	initializeSim()

	// get redis connection
	redisConn = redis.NewConnection(config.RedisHost, config.RedisPort, 0)

	// create clients map
	clients = cmap.New()

	// start server
	go loop()

	server()
}
