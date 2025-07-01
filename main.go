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
	"path/filepath"
	"strconv"
)

const usageText = `Usage:
	product <product> vendor <vendor> requests size [<request1>, <request2>...]`

// usage prints detailed usage and exits
func usage() {
	fmt.Printf(usageText)
	os.Exit(0)
}

type UsbArgs struct {
	Product  uint16
	Vendor   uint16
	Requests []string
}

func parseArgv() *UsbArgs {
	var vendor uint16
	var product uint16
	var requests []string

	for i := 0; i < len(os.Args)-2; i++ {
		switch os.Args[i] {
		case "vendor":
			if i+1 < len(os.Args) {
				i++
				value, err := strconv.ParseUint(os.Args[i], 10, 16)
				if err != nil {
					fmt.Println("Erro: valor para 'vendor' inválido.")
					usage()
				}

				vendor = uint16(value)
			} else {
				fmt.Println("Erro: valor para 'vendor' não fornecido.")
			}
		case "product":
			if i+1 < len(os.Args) {
				i++
				value, err := strconv.ParseUint(os.Args[i], 10, 16)
				if err != nil {
					fmt.Println("Erro: valor para 'product' inválido.")
					usage()
				}

				product = uint16(value)
			} else {
				fmt.Println("Erro: valor para 'product' não fornecido.")
			}
		case "requests":
			if i+1 < len(os.Args) {
				i++
				size, err := strconv.ParseUint(os.Args[i], 10, 16)
				if err != nil || size < 1 || i+int(size) >= len(os.Args) {
					fmt.Printf("Erro: valor size para 'requests' inválido. Valor: %s\n", os.Args[i])
					usage()
				}
				i++
				// requests size [<request1>, <request2>...]
				for j := i; j < i+int(size); j++ {
					requests = append(requests, os.Args[j])
				}
			}
		default:
			continue
		}
	}

	if vendor == 0 || product == 0 || len(requests) == 0 {
		fmt.Printf("Erro: valor para 'product': %d ou 'vendor': %d ou requests estão vazios.\n", vendor, product)
		usage()
	}

	// fmt.Printf("Vendor: %d, Product: %d, Requests: %v\n", vendor, product, requests)

	usbArgs := &UsbArgs{
		Vendor:   vendor,
		Product:  product,
		Requests: requests,
	}

	return usbArgs
}

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

func splitByLength(s string, size int) []string {
	var result []string
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		result = append(result, s[i:end])
	}
	return result
}

// The main function
func main() {
	fmt.Println("Iniciando o programa...")

	var err error

	usbArgs := parseArgv()
	// In RunCheck mode, list IPP-over-USB devices
	// If we are here, configuration is OK
	// InitLog.Info(0, "Configuration files: OK")
	InitLog.logger.SetLevels(1)

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
		// var list []UsbDeviceDesc
		// var buf bytes.Buffer

		for _, desc := range descs {
			if len(desc.IfDescs) > 0 {
				vendor := desc.IfDescs[0].Vendor
				product := desc.IfDescs[0].Product

				exePath, err := os.Executable()
				if err != nil {
					fmt.Printf("Erro ao obter o caminho do executável: %s.\n", err)
					return
				}

				// fmt.Printf("USB Vendor: %d, Product: %d\n", vendor, product)
				dir := filepath.Dir(exePath) // Obtém o diretório do executável
				filePath := filepath.Join(dir, "saida_do_go.txt")

				// fmt.Printf("diretorio: %s.\n", filePath)

				if (vendor == usbArgs.Vendor) && (product == usbArgs.Product) {
					responses, err := SendIppUsbRequest(desc, usbArgs.Requests)

					if err != nil {
						fmt.Printf("Erro ao coletar dados: %s.\n", err)
					} else {
						for _, response := range responses {
							file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
							if err != nil {
								panic(err)
							}

							// file.WriteString("teste\n\n")
							file.WriteString(response)
							file.Close()
						}
					}
				}
			}
		}
	}
}
