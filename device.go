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
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"

	// "os"
	"regexp"
	"strings"
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

type LoginResult struct {
	SessionCookie []*http.Cookie
	BaseURL       string
	LoginPath     string
}

func loginIfNeededv2(dev *Device, user, pass string) (*LoginResult, error) {
	baseURL := fmt.Sprintf("http://localhost:%d", dev.State.HTTPPort)

	resp0, err := dev.HTTPClient.Get(baseURL + "/")
	if err != nil || resp0.StatusCode != 200 {
		if resp0 != nil {
			resp0.Body.Close()
		}
		return nil, err
	}

	body0, err := io.ReadAll(resp0.Body)
	resp0.Body.Close()
	if err != nil {
		return nil, err
	}

	locationRegex := regexp.MustCompile(`\Wlocation\.href\s*=\s*['"]([^'"]*/)mainFrame\.cgi['"]`)
	matches := locationRegex.FindStringSubmatch(string(body0))
	if len(matches) < 2 {
		return nil, fmt.Errorf("failed to extract location URL")
	}
	lurl := matches[1]

	authURL := baseURL + lurl + "authForm.cgi"
	req1, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		return nil, err
	}
	req1.Header.Set("Cookie", "cookieOnOffChecker=on")

	resp1, err := dev.HTTPClient.Do(req1)
	if err != nil || resp1.StatusCode != 200 {
		if resp1 != nil {
			resp1.Body.Close()
		}
		return nil, fmt.Errorf("failed to get authForm.cgi")
	}

	body1, err := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read authForm.cgi response body: %w", err)
	}

	tokenRegex := regexp.MustCompile(`<input[^>]*type\s*=\s*['"]hidden['"][^>]*name\s*=\s*['"]wimToken['"][^>]*value\s*=\s*['"]([^'"]*)['"]`)
	tokenMatches := tokenRegex.FindStringSubmatch(string(body1))
	if len(tokenMatches) < 2 {
		return nil, fmt.Errorf("failed to extract wimToken")
	}
	token := tokenMatches[1]

	var cookies []string
	for _, cookie := range resp1.Cookies() {
		cookies = append(cookies, fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
	}
	cookieHeader := strings.Join(cookies, "; ")

	// Step 3: POST login form
	formData := url.Values{
		"wimToken":      {token},
		"userid_work":   {""},
		"userid":        {base64.StdEncoding.EncodeToString([]byte(user))},
		"password_work": {""},
		"password":      {base64.StdEncoding.EncodeToString([]byte(pass))},
		"open":          {""},
	}

	loginURL := baseURL + lurl + "login.cgi"
	req2, err := http.NewRequest("POST", loginURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("Cookie", cookieHeader)

	// fmt.Printf("Req2 headers: %s\n", req2.Header)

	dev.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp2, err := dev.HTTPClient.Do(req2)
	if err != nil {
		return nil, err
	}

	_, err = io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if resp2.StatusCode != 302 {
		return nil, fmt.Errorf("unexpected status code: %d", resp2.StatusCode)
	}

	location := resp2.Header.Get("Location")
	// fmt.Printf("Location header: %s\n", location)

	mainFrameRegex := regexp.MustCompile(`/mainFrame\.cgi$`)
	if !mainFrameRegex.MatchString(location) {
		return nil, fmt.Errorf("unexpected location header: %s", location)
	}

	var cookies2 []*http.Cookie
	// fmt.Printf("Cookies recebidos do post de autenticacao a serem usados:\n")
	for _, cookie := range resp2.Cookies() {
		if cookie.Name == "wimsesid" || cookie.Name == "cookieOnOffChecker" {
			cookies2 = append(cookies2, &http.Cookie{
				Name:   cookie.Name,
				Value:  cookie.Value,
				Path:   "/",
				Domain: "localhost",
			})
			// fmt.Printf("- %s: %s\n", cookie.Name, cookie.Value)
		}
	}

	// print headers
	// fmt.Printf("Response Headers: %v\n", resp2.Header)

	session := &LoginResult{
		SessionCookie: cookies2,
		BaseURL:       baseURL,
		LoginPath:     lurl,
	}

	for _, cookie := range resp2.Cookies() {
		if cookie.Name == "wimsesid" {
			numericRegex := regexp.MustCompile(`^\d+$`)
			if numericRegex.MatchString(cookie.Value) {
				return session, nil
			}
		}
	}

	return session, fmt.Errorf("wimsesid cookie not found or invalid")
}

// SendIppUsbRequest creates new Device object
func SendIppUsbRequest(desc UsbDeviceDesc, requests []string) ([]string, error) {
	dev := &Device{
		UsbAddr: desc.UsbAddr,
	}

	var err error
	var info UsbDeviceInfo
	var quirks Quirks
	var responses []string
	// var loginResult *LoginResult

	// file, err := os.Create("requests.txt")
	// if err != nil {
	// 	return nil, err
	// }
	// defer file.Close()

	// Escreve cada linha do slice no arquivo
	// _, err = file.WriteString(strings.Join(requests, "\n"))
	// if err != nil {
	// 	return nil, err
	// }

	// Create USB transport
	dev.UsbTransport, err = NewUsbTransport(desc)
	if err != nil {
		return nil, err
	}

	// file.WriteString("passou do NewUsbTransport\n")

	// Obtain quirks
	quirks = dev.UsbTransport.Quirks()

	// file.WriteString("passou do Quirks\n")

	// Obtain device's logger
	dev.Log = dev.UsbTransport.Log()

	// file.WriteString("passou do Log\n")

	// Obtain device info and derived information.
	info = dev.UsbTransport.UsbDeviceInfo()
	canPrint := info.BasicCaps&UsbIppBasicCapsPrint != 0

	// file.WriteString("passou do UsbDeviceInfo\n")

	// Load persistent state
	dev.State = LoadDevState(info.Ident(), info.Comment())

	// file.WriteString("passou do LoadDevState\n")

	// Create HTTP client for local queries with cookie support
	// jar, err := cookiejar.New(nil)
	// if err != nil {
	// 	return nil, fmt.Errorf("erro ao criar cookie jar: %w", err)
	// }

	dev.HTTPClient = &http.Client{
		Transport: dev.UsbTransport,
		// Jar:       jar, // Adiciona suporte a cookies
	}

	// file.WriteString("passou do HTTPListen\n")

	// Configure transport for init
	dev.UsbTransport.SetTimeout(quirks.GetInitTimeout())

	// file.WriteString("passou do NewHTTPProxy\n")

	// Realizar login
	// loginResult, err = loginIfNeededv2(dev, "admin", "Caiu2020")
	// if err != nil {
	// 	goto ERROR
	// }

	// file.WriteString("passou do loginIfNeededv2\n")

	// Log the wimToken
	// fmt.Printf("Cookies from login: %s\n", loginResult.SessionCookie)

	// [http://localhost:%d/web/guest/es/websys/status/getUnificationCounter.cgi]
	for _, request := range requests {
		uri := fmt.Sprintf(request, dev.State.HTTPPort)
		fmt.Printf("Uri: %s\n", uri)

		// Realizar a requisição HTTP
		req1, err := http.NewRequest("GET", uri, nil)
		if err != nil {
			// file.WriteString("Falhou no NewRequest\n")
			err = fmt.Errorf("failed to create request: %w", err)
			goto ERROR
		}

		// referer := loginResult.BaseURL + loginResult.LoginPath + "topPage.cgi"

		// req1.Header.Set("Referer", referer)
		// req1.Header.Set("Accept-Language", "es")
		// req1.Header.Set("Accept-Encoding", "gzip, deflate")
		// req1.Header.Set("Upgrade-Insecure-Requests", "1")
		// req1.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")

		// //cookie de resposta do login
		// for _, cookie := range loginResult.SessionCookie {
		// 	// fmt.Fprintf(file, "%s=%s\n", cookie.Name, cookie.Value)
		// 	req1.AddCookie(cookie)
		// }

		value, err := dev.HTTPClient.Do(req1)

		canRetry := ErrIsEOF(err)

		if err != nil {
			err = fmt.Errorf("HTTP Error for request ...URI...: %s - error: %s", request, err)

			if canRetry && canPrint && quirks.GetInitRetryPartial() {
				dev.Log.Begin().
					Info(' ', "Printer not ready (HTTP status %d)", value.StatusCode).
					Info(' ', "Retrying due to the %q quirk", QuirkNmInitRetryPartial).
					Commit()

				err = ErrPartialInit
			}

			goto ERROR
		}

		// Decode IPP response message
		respData, err := io.ReadAll(value.Body)
		if err != nil {
			err = fmt.Errorf("HTTP Error for request: %s - error: %s", request, err)
			goto ERROR
		}

		value.Body.Close()

		responses = append(responses, string(respData))
		fmt.Printf("Requisição concluída com sucesso para: %s\n", request)
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
