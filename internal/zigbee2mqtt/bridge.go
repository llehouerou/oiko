// Package zigbee2mqtt connects Oiko to a zigbee2mqtt instance over MQTT.
package zigbee2mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"

	"github.com/llehouerou/oiko/bridge"
)

func init() { bridge.Register(bridge.Module{Type: "zigbee2mqtt", New: open}) }

// Config is a zigbee2mqtt Bridge's section of Oiko's configuration.
type Config struct {
	// Broker is the MQTT broker zigbee2mqtt publishes to, credentials
	// included if any: mqtt://user:password@host:1883.
	Broker    string `json:"broker"`
	BaseTopic string `json:"baseTopic"` // zigbee2mqtt's, "zigbee2mqtt" by default
}

// Bridge is a zigbee2mqtt instance.
type Bridge struct {
	broker   *url.URL
	base     string // base topic
	clientID string
	marker   string // our own topic, published once subscribed: it comes back after the retained state
	home     bridge.Port

	mu        sync.RWMutex
	conn      *autopaho.ConnectionManager
	byAddress map[string]*deviceBinding
	byTopic   map[string]*deviceBinding
	online    bool // zigbee2mqtt says it is
	marked    bool // the marker came back
}

func open(env bridge.Env) (bridge.Bridge, error) {
	c := Config{BaseTopic: "zigbee2mqtt"}
	if err := json.Unmarshal(env.Config, &c); err != nil {
		return nil, fmt.Errorf("zigbee2mqtt: %w", err)
	}
	if c.Broker == "" {
		return nil, errors.New("zigbee2mqtt: broker is required")
	}
	broker, err := url.Parse(c.Broker)
	if err != nil {
		return nil, fmt.Errorf("zigbee2mqtt: broker: %w", err)
	}
	return newBridge(c.BaseTopic, broker), nil
}

func newBridge(baseTopic string, broker *url.URL) *Bridge {
	id := "oiko-" + uuid.New().String() // unique: instances sharing a broker must not evict each other
	return &Bridge{broker: broker, base: baseTopic, clientID: id, marker: "oiko/" + id + "/replayed"}
}

// settle notes zigbee2mqtt online or not, or the marker back, and tells Oiko
// the Bridge has replayed once both hold: zigbee2mqtt is online and the
// broker has replayed its retained state.
func (b *Bridge) settle(note func()) {
	b.mu.Lock()
	note()
	done := b.online && b.marked
	b.mu.Unlock()
	if done {
		b.home.Replayed()
	}
}

// Run connects to the broker and feeds Oiko through p until ctx is
// cancelled.
func (b *Bridge) Run(ctx context.Context, p bridge.Port) {
	b.home = p
	broker := b.broker
	cfg := autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{broker},
		KeepAlive:                     20,
		CleanStartOnInitialConnection: true,
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			log.Printf("mqtt: connected to %s", broker.Host)
			go func() {
				// NoLocal: don't receive our own /set publishes back. The broker
				// queues the retained state on subscribing, so the marker comes
				// back after it.
				// ponytail: MQTT orders messages per topic only; this holds for
				// Mosquitto, and main's 10 s cap bounds a broker it doesn't.
				sub := &paho.Subscribe{Subscriptions: []paho.SubscribeOptions{{Topic: b.base + "/#", QoS: 1, NoLocal: true}, {Topic: b.marker, QoS: 1}}}
				if _, err := cm.Subscribe(ctx, sub); err != nil {
					log.Printf("mqtt: subscribe: %v", err)
					return
				}
				if _, err := cm.Publish(ctx, &paho.Publish{Topic: b.marker, QoS: 1, Payload: []byte("replayed")}); err != nil {
					log.Printf("mqtt: publish: %v", err)
				}
			}()
		},
		OnConnectionDown: func() bool {
			log.Printf("mqtt: connection lost")
			p.SetOnline(false)
			return true
		},
		OnConnectError: func(err error) { log.Printf("mqtt: %v", err) },
		ClientConfig: paho.ClientConfig{
			ClientID: b.clientID,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){
				func(pr paho.PublishReceived) (bool, error) {
					b.handle(pr.Packet.Topic, pr.Packet.Payload, pr.Packet.Retain)
					return true, nil
				},
			},
		},
	}
	if u := broker.User; u != nil {
		password, _ := u.Password()
		cfg.SetUsernamePassword(u.Username(), []byte(password))
	}
	conn, err := autopaho.NewConnection(ctx, cfg)
	if err != nil { // a configuration autopaho refuses: the Bridge stays offline
		log.Printf("mqtt: %v", err)
		return
	}
	b.mu.Lock()
	b.conn = conn
	b.mu.Unlock()
	<-conn.Done()
}

// handle routes a message; retained marks a replay of an earlier message by
// the broker on subscription.
func (b *Bridge) handle(topic string, payload []byte, retained bool) {
	if topic == b.marker {
		b.settle(func() { b.marked = true })
		return
	}
	rest, ok := strings.CutPrefix(topic, b.base+"/")
	if !ok || len(payload) == 0 {
		return
	}
	switch {
	case rest == "bridge/state":
		var s struct{ State string }
		if err := json.Unmarshal(payload, &s); err == nil {
			b.home.SetOnline(s.State == "online")
			b.settle(func() { b.online = s.State == "online" })
		}
	case rest == "bridge/devices":
		b.syncDevices(payload)
	case strings.HasPrefix(rest, "bridge/"):
	default:
		name, isAvailability := strings.CutSuffix(rest, "/availability")
		b.mu.RLock()
		d := b.byTopic[name]
		b.mu.RUnlock()
		if d == nil { // groups, /set and /get echoes, unknown devices
			return
		}
		if isAvailability {
			var s struct{ State string }
			if err := json.Unmarshal(payload, &s); err == nil {
				b.home.SetAvailability(d.address, availability(s.State))
			}
			return
		}
		var state map[string]any
		if err := json.Unmarshal(payload, &state); err != nil {
			log.Printf("zigbee2mqtt: %s: %v", topic, err)
			return
		}
		b.home.Report(d.address, d.readings(state, !retained), reportedAt(state))
	}
}

// reportedAt is when the device sent the message: zigbee2mqtt's last_seen
// (advanced.last_seen: ISO_8601), which stays true for replayed messages;
// otherwise now.
func reportedAt(state map[string]any) time.Time {
	if s, ok := state["last_seen"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t
		}
	}
	return time.Now()
}

func availability(s string) bridge.Availability {
	switch s {
	case "online":
		return bridge.Online
	case "offline":
		return bridge.Offline
	}
	return bridge.Unknown
}

func (b *Bridge) syncDevices(payload []byte) {
	var list []device
	if err := json.Unmarshal(payload, &list); err != nil {
		log.Printf("zigbee2mqtt: bridge/devices: %v", err)
		return
	}
	var devices []bridge.Device
	byAddress := map[string]*deviceBinding{}
	byTopic := map[string]*deviceBinding{}
	for _, d := range list {
		if d.Type == "Coordinator" {
			continue
		}
		dev, binding := derive(d)
		devices = append(devices, dev)
		byAddress[binding.address] = binding
		byTopic[binding.topic] = binding
	}
	b.mu.Lock()
	b.byAddress, b.byTopic = byAddress, byTopic
	b.mu.Unlock()
	b.home.SyncDevices(devices)
}

// Send publishes values to the device's /set topic, with zigbee2mqtt's
// transition option (in seconds) when a fade is requested.
func (b *Bridge) Send(ctx context.Context, address, function string, values map[string]any, transition time.Duration) error {
	b.mu.RLock()
	d, conn := b.byAddress[address], b.conn
	b.mu.RUnlock()
	if d == nil || conn == nil {
		return fmt.Errorf("zigbee2mqtt: device %s unavailable", address)
	}
	payload, err := d.setPayload(function, values)
	if err != nil {
		return err
	}
	if transition > 0 {
		payload["transition"] = transition.Seconds()
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = conn.Publish(ctx, &paho.Publish{Topic: b.base + "/" + d.topic + "/set", QoS: 1, Payload: body})
	return err
}
