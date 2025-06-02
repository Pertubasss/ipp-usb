/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Device object brings all parts together
 */

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	// "regexp"
	// "strings"
)

// Device object brings all parts together, namely:
//   - HTTP proxy server
//   - USB-backed http.Transport
//   - DNS-SD advertiser
//
// There is one instance of Device object per USB device
type Device struct {
	UsbAddr        UsbAddr         // Device's USB address
	State          *DevState       // Persistent state
	HTTPClient     *http.Client    // HTTP client for internal queries
	HTTPProxy      *HTTPProxy      // HTTP proxy
	UsbTransport   *UsbTransport   // Backing USB transport
	DNSSdPublisher *DNSSdPublisher // DNS-SD publisher
	Log            *Logger         // Device's logger
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func NewDevice(desc UsbDeviceDesc) (*Device, error) {
	dev := &Device{
		UsbAddr: desc.UsbAddr,
	}

	return dev, nil
}

// SendIppUsbRequest creates new Device object
func SendIppUsbRequest(desc UsbDeviceDesc, requests []string) ([]string, error) {
	dev := &Device{
		UsbAddr: desc.UsbAddr,
	}

	// fmt.Println("Teste printLN")

	var err error
	var info UsbDeviceInfo
	var listener net.Listener
	// var log *LogMessage
	var quirks Quirks
	var responses []string

	// Create USB transport
	dev.UsbTransport, err = NewUsbTransport(desc)
	if err != nil {
		return nil, err
	}

	// fmt.Println("apos NewUsbTransport")

	// Obtain quirks
	quirks = dev.UsbTransport.Quirks()

	// fmt.Println("apos Quirks")

	// Obtain device's logger
	// dev.Log = dev.UsbTransport.Log()

	// Obtain device info and derived information.
	info = dev.UsbTransport.UsbDeviceInfo()
	canPrint := info.BasicCaps&UsbIppBasicCapsPrint != 0

	// fmt.Println("apos UsbDeviceInfo")

	// Load persistent state
	dev.State = LoadDevState(info.Ident(), info.Comment())

	// fmt.Println("apos LoadDevState")

	// Create HTTP client for local queries
	dev.HTTPClient = &http.Client{
		Transport: dev.UsbTransport,
	}

	// Create net.Listener
	listener, err = dev.State.HTTPListen()
	if err != nil {
		goto ERROR
	}

	// fmt.Println("apos HTTPListen")

	// Configure transport for init
	dev.UsbTransport.SetTimeout(quirks.GetInitTimeout())

	// Create HTTP server
	dev.HTTPProxy = NewHTTPProxy(dev.Log, listener, dev.UsbTransport)

	// fmt.Println("apos NewHTTPProxy")

	// Obtain DNS-SD info for IPP
	// log = dev.Log.Begin()
	// defer log.Commit()

	// dev.Log.Debug(' ', "apos Begin")

	//uri := fmt.Sprintf("http://localhost:%d/main.asp?Lang=en-us", dev.State.HTTPPort)

	for _, request := range requests {
		// uri := fmt.Sprintf("http://localhost:%d/web/guest/es/websys/webArch/getStatus.cgi", dev.State.HTTPPort)
		uri := fmt.Sprintf(request, dev.State.HTTPPort)
		fmt.Printf("Uri: %s", uri)
		value, err := dev.HTTPClient.Get(uri)

		if err != nil {
			err = fmt.Errorf("HTTP Error for request: %s - error: %s: %s", request, err)
			canRetry := value.StatusCode != 0 || ErrIsEOF(err)

			if canRetry && canPrint && quirks.GetInitRetryPartial() {
				dev.Log.Begin().
					Info(' ', "Printer not ready (HTTP status %d)",
						value.StatusCode).
					Info(' ', "Retrying due to the %q quirk",
						QuirkNmInitRetryPartial).
					Commit()

				err = ErrPartialInit
			}

			goto ERROR
		}

		defer value.Body.Close()

		// Decode IPP response message
		respData, err := ioutil.ReadAll(value.Body)
		if err != nil {
			err = fmt.Errorf("Decode IPP response messagem: %s - error: %s: %s", request, err)
			goto ERROR
		}

		responses = append(responses, string(respData))
		fmt.Println("Requisição concluida com sucesso para: %s", request)
		// dev.Log.Debug(' ', "vai printar o retorno do equipamento:")
		// fmt.Println(string(respData))
	}

	return responses, nil

ERROR:
	if dev.HTTPProxy != nil {
		dev.HTTPProxy.Close()
	}

	if dev.UsbTransport != nil {
		reset := true
		switch err {
		case ErrUnusable, ErrPartialInit:
			reset = false
		}
		dev.UsbTransport.Close(reset)
	}

	if listener != nil {
		listener.Close()
	}

	return nil, err
}

// Shutdown gracefully shuts down the device. If provided context
// expires before the shutdown is complete, Shutdown returns the
// context's error
func (dev *Device) Shutdown(ctx context.Context) error {
	if dev.DNSSdPublisher != nil {
		dev.DNSSdPublisher.Unpublish()
		dev.DNSSdPublisher = nil
	}

	if dev.HTTPProxy != nil {
		dev.HTTPProxy.Close()
		dev.HTTPProxy = nil
	}

	if dev.UsbTransport != nil {
		return dev.UsbTransport.Shutdown(ctx)
	}

	return nil
}

// Close the Device
func (dev *Device) Close() {
	if dev.DNSSdPublisher != nil {
		dev.DNSSdPublisher.Unpublish()
		dev.DNSSdPublisher = nil
	}

	if dev.HTTPProxy != nil {
		dev.HTTPProxy.Close()
		dev.HTTPProxy = nil
	}

	if dev.UsbTransport != nil {
		dev.UsbTransport.Close(false)
		dev.UsbTransport = nil
	}
}
