/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * The main function
 */

package main

import (
	"bytes"
	"fmt"
	"os"
	"sort"
)

const usageText = `Usage:
	 %s mode [options]
 
 Modes are:
	 standalone  - run forever, automatically discover IPP-over-USB
				   devices and serve them all
	 udev        - like standalone, but exit when last IPP-over-USB
				   device is disconnected
	 debug       - logs duplicated on console, -bg option is
				   ignored
	 check       - check configuration and exit
	 status      - print ipp-usb status and exit
 
 Options are
	 -bg         - run in background (ignored in debug mode)
 `

// RunMode represents the program run mode
type RunMode int

// Run modes:
//
//	RunStandalone - run forever, automatically discover IPP-over-USB
//	                devices and serve them all
//	RunUdev       - like RunStandalone, but exit when last IPP-over-USB
//	                device is disconnected
//	RunDebug      - logs duplicated on console, -bg option is ignored
//	RunCheck      - check configuration and exit
//	RunStatus     - print ipp-usb status and exit
const (
	RunDefault RunMode = iota
	RunStandalone
	RunUdev
	RunDebug
	RunCheck
	RunStatus
)

// String returns RunMode name
func (m RunMode) String() string {
	switch m {
	case RunDefault:
		return "default"
	case RunStandalone:
		return "standalone"
	case RunUdev:
		return "udev"
	case RunDebug:
		return "debug"
	case RunCheck:
		return "check"
	case RunStatus:
		return "status"
	}

	return fmt.Sprintf("unknown (%d)", int(m))
}

// RunParameters represents the program run parameters
type RunParameters struct {
	Mode       RunMode // Run mode
	Background bool    // Run in background
}

// usage prints detailed usage and exits
func usage() {
	fmt.Printf(usageText, os.Args[0])
	os.Exit(0)
}

// usage_error prints usage error and exits
func usageError(format string, args ...interface{}) {
	if format != "" {
		fmt.Printf(format+"\n", args...)
	}

	fmt.Printf("Try %s -h for more information\n", os.Args[0])
	os.Exit(1)
}

// parseArgv parses program parameters. In a case of usage error,
// it prints a error message and exits
func parseArgv() (params RunParameters) {
	// Catch panics to log
	defer func() {
		v := recover()
		if v != nil {
			Log.Panic(v)
		}
	}()

	// For now, default mode is debug mode. It may change in a future
	params.Mode = RunDebug

	modes := 0
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-h", "-help", "--help":
			usage()
		case "standalone":
			params.Mode = RunStandalone
			modes++
		case "udev":
			params.Mode = RunUdev
			modes++
		case "debug":
			params.Mode = RunDebug
			modes++
		case "check":
			params.Mode = RunCheck
			modes++
		case "status":
			params.Mode = RunStatus
			modes++
		case "-bg":
			params.Background = true
		default:
			usageError("Invalid argument %s", arg)
		}
	}

	if modes > 1 {
		usageError("Conflicting run modes")
	}

	if params.Mode == RunDebug {
		params.Background = false
	}

	return
}

// printStatus prints status of running ipp-usb daemon, if any
func printStatus() {
	// Fetch status
	text, err := StatusRetrieve()

	if err != nil {
		InitLog.Info(0, "%s", err)
		return
	}

	// Split into lines
	text = bytes.Trim(text, "\n")
	lines := bytes.Split(text, []byte("\n"))

	// Strip empty lines at the end
	for len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[0 : len(lines)-1]
	}

	// Write to log, line by line
	for _, line := range lines {
		InitLog.Info(0, "%s", line)
	}
}

// The main function
func main() {
	var err error

	// In RunCheck mode, list IPP-over-USB devices
	// If we are here, configuration is OK
	InitLog.Info(0, "Configuration files: OK")

	var descs map[UsbAddr]UsbDeviceDesc
	err = UsbInit(true)
	if err == nil {
		descs, err = UsbGetIppOverUsbDeviceDescs()
	}

	if err != nil {
		InitLog.Info(0, "Can't read list of USB devices: %s", err)
	} else if descs == nil || len(descs) == 0 {
		InitLog.Info(0, "No IPP over USB devices found")
	} else {
		// Repack into the sorted list
		var list []UsbDeviceDesc
		var buf bytes.Buffer

		for _, desc := range descs {
			NewDevice(desc)
			list = append(list, desc)
		}
		sort.Slice(list, func(i, j int) bool {
			return list[i].UsbAddr.Less(list[j].UsbAddr)
		})

		InitLog.Info(0, "IPP over USB devices:")
		InitLog.Info(0, " Num  Device              Vndr:Prod  Model")
		for i, dev := range list {
			buf.Reset()
			fmt.Fprintf(&buf, "%3d. %s", i+1, dev.UsbAddr)
			if info, err := dev.GetUsbDeviceInfo(); err == nil { //não está capturando corretamente no windows para o equipamento Ricoh
				fmt.Fprintf(&buf, "  %4.4x:%.4x  %q",
					info.Vendor, info.Product, info.MfgAndProduct)
			}

			InitLog.Info(0, " %s", buf.String())
		}
	}

}
