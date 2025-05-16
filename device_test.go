/* ipp-usb - HTTP reverse proxy, backed by IPP-over-USB connection to device
 *
 * Copyright (C) 2020 and up by Alexander Pevzner (pzz@apevzner.com)
 * See LICENSE for license terms and conditions
 *
 * Device object brings all parts togethe

package main_test

import(
	"github.com/OpenPrinting/ipp-usb"
	"testing"
)

func TestSendIppUsbRequest(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		desc     main.UsbDeviceDesc
		requests []string
		want     []string
		wantErr  bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := main.SendIppUsbRequest(tt.desc, tt.requests)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("SendIppUsbRequest() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("SendIppUsbRequest() succeeded unexpectedly")
			}
			// TODO: update the condition below to compare got with tt.want.
			if true {
				t.Errorf("SendIppUsbRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}
