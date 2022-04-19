package main

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
)

func getProxyClient(scheme string, host string, port int, user string, pass string) *http.Client {
	proxyString := fmt.Sprintf(
		"%s://%s:%s@%s:%d",
		scheme, user, pass, host, port,
	)
	proxyURL, err := url.Parse(proxyString)
	if err != nil {
		return nil
	}
	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}
	return httpClient
}

func getTemplate(config Config) (*template.Template, error) {
	tmpl, err := template.ParseFiles(config.NotificationTemplate)
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}
