package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/brutella/hc"
	"github.com/brutella/hc/accessory"
	"github.com/brutella/hc/characteristic"
	hclog "github.com/brutella/hc/log"
	"github.com/brutella/hc/service"
	"github.com/joeshaw/leaf"
	"github.com/peterbourgon/ff/v3"
)

type Leaf struct {
	session *leaf.Session

	accessory *accessory.Accessory
	battery   *service.BatteryService
	climate   *service.Switch
	charge    *service.Switch
}

type config struct {
	storagePath    string
	username       string
	password       string
	country        string
	accessoryName  string
	homekitPIN     string
	updateInterval time.Duration
	debug          bool
}

func main() {
	var cfg config

	fs := flag.NewFlagSet("leaf-homekit", flag.ExitOnError)
	fs.StringVar(
		&cfg.storagePath,
		"storage-path",
		filepath.Join(os.Getenv("HOME"), ".homecontrol", "leaf"),
		"Storage path for information about the HomeKit accessory",
	)
	fs.StringVar(&cfg.username, "username", "", "Nissan username")
	fs.StringVar(&cfg.password, "password", "", "Nissan password")
	fs.StringVar(&cfg.country, "country", "US", "Leaf country")
	fs.StringVar(&cfg.accessoryName, "accessory-name", "", "HomeKit accessory name")
	fs.StringVar(&cfg.homekitPIN, "homekit-pin", "00102003", "HomeKit pairing PIN")
	fs.DurationVar(&cfg.updateInterval, "update-interval", 15*time.Minute, "How often to update battery status")
	fs.BoolVar(&cfg.debug, "debug", false, "Enable debug mode")
	_ = fs.String("config", "", "Config file")

	ff.Parse(fs, os.Args[1:],
		ff.WithEnvVarPrefix("LEAF"),
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ff.PlainParser),
	)

	if cfg.username == "" || cfg.password == "" {
		log.Fatal("username and password required")
	}

	s := &leaf.Session{
		Username: cfg.username,
		Password: cfg.password,
		Country:  cfg.country,
		Debug:    cfg.debug,
	}

	if cfg.debug {
		hclog.Debug.Enable()
	}

	log.Println("Connecting to NissanConnect service")
	vehicle, batteryRecord, _, err := s.Login()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Found %s %s, VIN %s", vehicle.ModelYear, vehicle.ModelName, vehicle.VIN)

	info := accessory.Info{
		Name:         vehicle.Nickname,
		Manufacturer: "Nissan",
		Model:        fmt.Sprintf("%s %s", vehicle.ModelYear, vehicle.ModelName),
		SerialNumber: vehicle.VIN,
	}

	if cfg.accessoryName != "" {
		info.Name = cfg.accessoryName
	}

	l := &Leaf{
		session:   s,
		accessory: accessory.New(info, accessory.TypeOther),
		battery:   service.NewBatteryService(),
		climate:   service.NewSwitch(),
		charge:    service.NewSwitch(),
	}

	l.accessory.AddService(l.battery.Service)
	l.accessory.AddService(l.climate.Service)
	l.accessory.AddService(l.charge.Service)

	l.setBatteryCharacteristics(batteryRecord)

	// It's too slow to pull battery info on demand, and too taxing on
	// the car's 12V battery.  Update the value in a loop instead.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.updateBatteryLoop(ctx, cfg.updateInterval)

	// We have to use normal switches for these, because HomeKit doesn't
	// have stateless buttons

	n := characteristic.NewName()
	n.SetValue("Climate Control")
	l.climate.AddCharacteristic(n.Characteristic)
	l.climate.On.OnValueRemoteUpdate(l.sendClimateRequest)
	l.climate.On.OnValueRemoteGet(func() bool { return false })

	n = characteristic.NewName()
	n.SetValue("Charging")
	l.charge.AddCharacteristic(n.Characteristic)
	l.charge.On.OnValueRemoteUpdate(l.sendChargingRequest)
	l.charge.On.OnValueRemoteGet(func() bool { return false })

	hcConfig := hc.Config{
		Pin:         cfg.homekitPIN,
		StoragePath: cfg.storagePath,
	}

	t, err := hc.NewIPTransport(hcConfig, l.accessory)
	if err != nil {
		log.Fatal(err)
	}

	hc.OnTermination(func() {
		cancel()
		<-t.Stop()
	})

	log.Println("Starting transport...")
	t.Start()
}

func (l *Leaf) setBatteryCharacteristics(br *leaf.BatteryRecords) {
	l.battery.BatteryLevel.SetValue(br.BatteryStatus.SOC.Value)

	lowBatt := characteristic.StatusLowBatteryBatteryLevelNormal
	if br.BatteryStatus.SOC.Value <= 20 {
		lowBatt = characteristic.StatusLowBatteryBatteryLevelLow
	}
	l.battery.StatusLowBattery.SetValue(lowBatt)

	status := characteristic.ChargingStateNotCharging
	if br.BatteryStatus.BatteryChargingStatus.IsCharging() {
		status = characteristic.ChargingStateCharging
	}
	l.battery.ChargingState.SetValue(status)

	log.Printf("Battery Level: %d%%  Charging: %s", br.BatteryStatus.SOC.Value, br.BatteryStatus.BatteryChargingStatus)
}

func (l *Leaf) updateBatteryLoop(ctx context.Context, interval time.Duration) {
	log.Printf("Entering battery update loop, updating every %v", interval)
	defer log.Println("Exited battery update loop")

	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			log.Println("Updating battery information")
			br, _, err := l.session.ChargingStatus()
			if err != nil {
				log.Printf("Error updating battery info: %v", err)
			} else {
				l.setBatteryCharacteristics(br)
			}
		}
	}
}

func (l *Leaf) sendChargingRequest(on bool) {
	// These are stateless switches, always set them to off
	defer func() {
		time.Sleep(1 * time.Second)
		l.charge.On.SetValue(false)
	}()

	if !on {
		return
	}

	log.Println("Sending charging request...")
	if err := l.session.StartCharging(); err != nil {
		log.Printf("Unable to send charging request: %v", err)
	}
	log.Println("Successfully sent charging request")
}

func (l *Leaf) sendClimateRequest(on bool) {
	// These are stateless switches, always set them to off
	defer func() {
		time.Sleep(1 * time.Second)
		l.climate.On.SetValue(false)
	}()

	if !on {
		return
	}

	log.Println("Sending climate request...")
	if err := l.session.ClimateOn(); err != nil {
		log.Printf("Unable to send climate request: %v", err)
	}
	log.Println("Successfully sent climate request")
}
