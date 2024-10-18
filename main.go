/*
 * @Author: FunctionSir
 * @License: AGPLv3
 * @Date: 2024-10-14 21:59:55
 * @LastEditTime: 2024-10-18 21:39:56
 * @LastEditors: FunctionSir
 * @Description: -
 * @FilePath: /RobKillSwitch/main.go
 */

package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/karalabe/usb"
	"gopkg.in/ini.v1"
)

type VOID struct{}

const SHELL_UNIX_LIKE string = "/bin/sh"
const SHELL_WINDOWS string = "C:/Windows/System32/cmd.exe"

const CONF_SECTION string = "DEFAULT"
const ANY_ID uint16 = 0

type DeviceTriplet struct {
	VendorID  uint16
	ProductID uint16
	B64Serial string
}

type Conf struct {
	DevicesList string
	Cmd         string
	ChkGap      int
}

func LogFatalln(s string) {
	c := color.New(color.FgHiRed, color.Bold)
	log.Fatalln(c.Sprint(s))
}

func LogWarnln(s string) {
	c := color.New(color.FgHiYellow)
	log.Println(c.Sprint(s))
}

func LogInfoln(s string) {
	c := color.New(color.FgHiGreen)
	log.Println(c.Sprint(s))
}

func FailOnNoSection(file *ini.File, section string) *ini.Section {
	if !file.HasSection(section) {
		LogFatalln("Error in config file: no section \"" + section + "\" found.")
	}
	return file.Section(section)
}

func FailOnNoKey(section *ini.Section, key string) *ini.Key {
	if !section.HasKey(key) {
		LogFatalln("Error in config file: no key \"" + key + "\" in section \"" + section.Name() + "\" found.")
	}
	return section.Key(key)
}

func getConf(path string) Conf {
	file, err := ini.Load(path)
	if err != nil {
		LogFatalln("Error occurred during reading config file: " + err.Error())
	}
	sec := FailOnNoSection(file, CONF_SECTION)
	deviceList := FailOnNoKey(sec, "DevicesList").String()
	cmd := FailOnNoKey(sec, "Cmd").String()
	chkGap := 100
	if sec.HasKey("ChkGap") {
		chkGap, err = strconv.Atoi(sec.Key("ChkGap").String())
		if err != nil {
			LogFatalln("Error occurred when parsing config key \"ChkGap\": " + err.Error())
		}
	}
	return Conf{deviceList, cmd, chkGap}
}

func getTriggers(path string) map[DeviceTriplet]bool {
	triggers := make(map[DeviceTriplet]bool)
	content, err := os.ReadFile(path)
	if err != nil {
		LogFatalln("Error occurred during reading device list: " + err.Error())
	}
	lines := strings.Split(string(content), "\n")
	for _, x := range lines {
		if len(x) == 0 || x[0] == '#' {
			continue
		}
		splited := strings.Split(x, ":")
		if len(splited) != 3 {
			LogFatalln("Error in device ID list detected: illegal entry")
		}
		vendorId, err := strconv.ParseUint(splited[0], 16, 16)
		if err != nil {
			LogFatalln("Error parsing vendor ID: " + err.Error())
		}
		deviceId, err := strconv.ParseUint(splited[1], 16, 16)
		if err != nil {
			LogFatalln("Error parsing device ID: " + err.Error())
		}
		device := DeviceTriplet{uint16(vendorId), uint16(deviceId), splited[2]}
		triggers[device] = false
	}
	return triggers
}

func ToDeviceTriplet(deviceInfo *usb.DeviceInfo) DeviceTriplet {
	return DeviceTriplet{deviceInfo.VendorID, deviceInfo.ProductID, base64.StdEncoding.EncodeToString([]byte(deviceInfo.Serial))}
}

func IsWindows() bool {
	return runtime.GOOS == "windows"
}

func cmdExecuter(shell string, cmd string) {
	firstArg := "-c"
	if IsWindows() {
		firstArg = "/c"
	}
	exec.Command(shell, firstArg, cmd).Run()
}

func main() {
	c := color.New(color.FgHiBlue)
	c.Println("RobKillSwitch [ Ver: 0.0.1 (ShirasuAzusa) ]")
	c.Println("By FunctionSir with love. This is a libre software under GPLv3.")
	if len(os.Args) <= 1 {
		LogFatalln("Error: no config file specified.")
	}
	confPath := os.Args[1]
	conf := getConf(confPath)
	if len(os.Args) >= 3 && os.Args[2] == "conf-mode" {
		fmt.Println("You are currently in config mode.")
		devices, err := usb.Enumerate(ANY_ID, ANY_ID)
		if err != nil {
			LogFatalln("Error: can not enumerate USB devices properly: " + err.Error())
		}
		fmt.Println("All USB devices:")
		for id, x := range devices {
			tmp := ToDeviceTriplet(&x)
			tmpSerial := strings.TrimSpace(x.Serial)
			if len(tmpSerial) >= 1 {
				tmpSerial += " "
			}
			fmt.Printf("[%d] %s %s %s(%x:%x:%s)\n", id, x.Manufacturer, x.Product, tmpSerial, tmp.VendorID, tmp.ProductID, tmp.B64Serial)
		}
		toWrite := ""
		fmt.Println("Input ID (in the []), use Enter as separator, input -1 to end:")
		var id int
		fmt.Scanf("%d", &id)
		for id != -1 {
			if id > len(devices)-1 || id <= -2 {
				fmt.Println("No such entry, try again.")
				fmt.Scanf("%d", &id)
				continue
			}
			tmp := ToDeviceTriplet(&devices[id])
			toWrite += fmt.Sprintf("%x:%x:%s\n", tmp.VendorID, tmp.ProductID, tmp.B64Serial)
			fmt.Scanf("%d", &id)
		}
		err = os.WriteFile(conf.DevicesList, []byte(toWrite), os.FileMode(0600))
		if err != nil {
			LogFatalln("Unable to write device list to file: " + err.Error())
		}
		LogInfoln("Successfully wrote device list to file.")
		os.Exit(0)
	}
	triggers := getTriggers(conf.DevicesList)
	shell := SHELL_UNIX_LIKE
	if IsWindows() {
		shell = SHELL_WINDOWS
	}
	for {
		devices, err := usb.Enumerate(ANY_ID, ANY_ID)
		if err != nil {
			LogWarnln("Warning: can not enumerate USB devices properly: " + err.Error())
			continue
		}
		cur := make(map[DeviceTriplet]VOID)
		for _, x := range devices {
			deviceTriplet := ToDeviceTriplet(&x)
			x, exists := triggers[deviceTriplet]
			if !exists {
				continue
			}
			cur[deviceTriplet] = VOID{}
			if !x {
				triggers[deviceTriplet] = true
			}
		}
		for key, seen := range triggers {
			_, exists := cur[key]
			if seen && !exists {
				LogWarnln("Kill switch triggered!")
				cmdExecuter(shell, conf.Cmd)
				triggers[key] = false
			}
		}
		time.Sleep(time.Duration(conf.ChkGap) * time.Millisecond)
	}
}
