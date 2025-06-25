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
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
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

// login web tentativa
func loginIfNeeded(dev *Device, username, password string) error {
	loginURL := fmt.Sprintf("http://localhost:%d/web/guest/es/websys/webArch/authForm.cgi", dev.State.HTTPPort)

	// Realizar a requisição HTTP
	// Criar requisição GET
	req, err := http.NewRequest("GET", loginURL, strings.NewReader(""))
	if err != nil {
		return fmt.Errorf("erro ao criar requisição de login: %w", err)
	}

	//validar sobre cabeçalhos!!!

	// Adicionar cookies manualmente
	req.AddCookie(&http.Cookie{
		Name:  "cookieOnOffChecker",
		Value: "on",
	})
	req.AddCookie(&http.Cookie{
		Name:  "risessionid",
		Value: "128319570606029", // Substitua pelo valor dinâmico, se necessário
	})
	req.AddCookie(&http.Cookie{
		Name:  "wimsesid",
		Value: "--",
	})

	// Enviar a requisição
	value, err := dev.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("erro ao enviar requisição de login: %w", err)
	}
	defer value.Body.Close()

	respData, err := ioutil.ReadAll(value.Body)
	if err != nil {
		err = fmt.Errorf("HTTP Error for request: %s - error: %s", value, err)
		return err
	}

	re := regexp.MustCompile(`name="wimToken" value="([^"]+)"`)

	// Encontrar o valor
	match := re.FindStringSubmatch(string(respData))
	var wimToken string
	if len(match) > 1 {
		wimToken = match[1] // Captura o valor do grupo 1
		fmt.Println("wimToken capturado:", wimToken)
	} else {
		fmt.Println("wimToken não encontrado")
	}

	b := value.Cookies()

	fmt.Println("Cookies recebidos do get:")
	var risession string = ""
	for _, cookie := range b {
		if cookie.Name == "risessionid" {
			risession = cookie.Value // Captura o valor do cookie risessionid
			fmt.Printf("- %s: %s\n", cookie.Name, cookie.Value)
		}
	}

	// fmt.Printf("Resposta do get de autenticacao: %s\n", string(respData))

	value.Body.Close()

	// Codificar username e password em Base64
	encodedUsername := base64.StdEncoding.EncodeToString([]byte(username))
	encodedPassword := base64.StdEncoding.EncodeToString([]byte(password))

	fmt.Printf("user criptografado: %s\n", encodedUsername)
	fmt.Printf("pass criptografado: %s\n", encodedPassword)

	fmt.Printf("user: %s\n", username)
	fmt.Printf("pass: %s\n", password)

	//Cria login
	form := url.Values{}
	form.Add("userid", encodedUsername)
	form.Add("password", encodedPassword)
	form.Add("wimtoken", wimToken)
	form.Add("userid_work", "")
	form.Add("password_work", "")
	form.Add("open", "")

	// Cria req Post
	//comentei pq PostForm nao deixa adicionar cookies!!!!!
	// resp, err := dev.HTTPClient.PostForm(loginURL, form)
	// if err != nil {
	// 	return fmt.Errorf("erro ao enviar formulário de login: %w", err)
	// }
	// defer resp.Body.Close()

	// Criar requisição POST
	req2, err := http.NewRequest("POST", loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("erro ao criar requisição de login: %w", err)
	}

	//validar sobre cabeçalhos!!!
	req2.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req2.Header.Set("Accept-Encoding", "gzip, deflate")
	req2.Header.Set("Accept-Language", "es")
	req2.Header.Set("Cache-Control", "max-age=0")
	req2.Header.Set("Connection", "keep-alive")
	req2.Header.Set("Content-Length", fmt.Sprintf("%d", len(form.Encode())))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("Origin", "http://192.168.10.99")
	req2.Header.Set("Referer", "http://192.168.10.99/web/guest/es/websys/webArch/login.cgi")
	req2.Header.Set("Upgrade-Insecure-Requests", "1")
	req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36 Edg/137.0.0.0")

	// Adicionar cookies manualmente
	req2.AddCookie(&http.Cookie{
		Name:  "cookieOnOffChecker",
		Value: "on",
	})
	req2.AddCookie(&http.Cookie{
		Name:  "risessionid",
		Value: risession, // Substitua pelo valor dinâmico, se necessário
	})
	req2.AddCookie(&http.Cookie{
		Name:  "wimsesid",
		Value: "--",
	})

	// Enviar a requisição
	resp, err := dev.HTTPClient.Do(req2)
	if err != nil {
		return fmt.Errorf("erro ao enviar requisição de login: %w", err)
	}

	// Iterar sobre os cabeçalhos
	fmt.Println("Cabeçalhos da resposta:")
	for key, values := range resp.Header {
		for _, value := range values {
			fmt.Printf("- %s: %s\n", key, value)
		}
	}

	fmt.Println("StatusCode: ", resp.StatusCode)
	fmt.Println("Status: ", resp.Status)

	defer resp.Body.Close()

	// Ler corpo da resposta
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("erro ao ler resposta do login: %w", err)
	}

	fmt.Println("Resposta após login:", string(bodyBytes))

	a := resp.Cookies()

	fmt.Println("Cookies recebidos após login:")
	for _, cookie := range a {
		fmt.Printf("- %s: %s\n", cookie.Name, cookie.Value)
	}

	if dev.HTTPClient.Jar != nil {

		cookies := dev.HTTPClient.Jar.Cookies(resp.Request.URL)
		fmt.Println("Cookies armazenados após login:")
		for _, cookie := range cookies {
			fmt.Printf("- %s: %s\n", cookie.Name, cookie.Value)
		}
	} else {
		fmt.Println("Nenhum cookie armazenado após login.")
	}

	//tentativa de validação mas acho que nao funciona
	if strings.Contains(string(bodyBytes), "Login") && strings.Contains(string(bodyBytes), "userid") {
		return fmt.Errorf("login falhou, página de login retornada novamente")
	}

	// Verificar o status HTTP
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login falhou, status HTTP: %d", resp.StatusCode)
	}

	// Se o login foi bem-sucedido
	fmt.Println("Login realizado com sucesso.")
	return nil
}

// SendIppUsbRequest creates new Device object
func SendIppUsbRequest(desc UsbDeviceDesc, requests []string) ([]string, error) {
	dev := &Device{
		UsbAddr: desc.UsbAddr,
	}

	var err error
	var info UsbDeviceInfo
	var listener net.Listener
	var quirks Quirks
	var responses []string

	// Create USB transport
	dev.UsbTransport, err = NewUsbTransport(desc)
	if err != nil {
		return nil, err
	}

	// Obtain quirks
	quirks = dev.UsbTransport.Quirks()

	// Obtain device's logger
	dev.Log = dev.UsbTransport.Log()

	// Obtain device info and derived information.
	info = dev.UsbTransport.UsbDeviceInfo()
	canPrint := info.BasicCaps&UsbIppBasicCapsPrint != 0

	// Load persistent state
	dev.State = LoadDevState(info.Ident(), info.Comment())

	// Create HTTP client for local queries with cookie support
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar cookie jar: %w", err)
	}

	dev.HTTPClient = &http.Client{
		Transport: dev.UsbTransport,
		Jar:       jar, // Adiciona suporte a cookies
	}

	// Create net.Listener
	listener, err = dev.State.HTTPListen()
	if err != nil {
		goto ERROR
	}

	// Configure transport for init
	dev.UsbTransport.SetTimeout(quirks.GetInitTimeout())

	// Create HTTP server
	dev.HTTPProxy = NewHTTPProxy(dev.Log, listener, dev.UsbTransport)

	// Realizar login
	err = loginIfNeeded(dev, "admin", "Caiu2020")
	if err != nil {
		goto ERROR
	}

	for _, request := range requests {
		uri := fmt.Sprintf(request, dev.State.HTTPPort)
		fmt.Printf("Uri: %s\n", uri)

		// Realizar a requisição HTTP
		value, err := dev.HTTPClient.Get(uri)
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
		respData, err := ioutil.ReadAll(value.Body)
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
