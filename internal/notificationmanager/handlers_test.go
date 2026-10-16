package notificationmanager

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/notification"
)

func TestWebhookImageURLFormats(t *testing.T) {
	oldLogger := helpers.AppLogger
	helpers.AppLogger = &helpers.QLogger{Logger: log.New(io.Discard, "", 0)}
	defer func() { helpers.AppLogger = oldLogger }()

	const imageURL = "https://media.themoviedb.org/t/p/w600_and_h900_face/xqKoHK1kOt6jvoMynLt4bEH1co0.jpg"
	formats := []struct {
		name     string
		template string
		getImage func(string) (string, error)
	}{
		{
			name:     "json",
			template: `{"image":"{{image}}"}`,
			getImage: func(body string) (string, error) {
				var payload map[string]string
				if err := json.Unmarshal([]byte(body), &payload); err != nil {
					return "", err
				}
				return payload["image"], nil
			},
		},
		{
			name:     "form",
			template: "image={{image}}",
			getImage: func(body string) (string, error) {
				values, err := url.ParseQuery(body)
				return values.Get("image"), err
			},
		},
		{
			name:     "text",
			template: "image={{image}}",
			getImage: func(body string) (string, error) {
				return strings.TrimPrefix(body, "image="), nil
			},
		},
	}

	for _, method := range []string{http.MethodPost, http.MethodGet} {
		for _, format := range formats {
			for _, image := range []string{imageURL, ""} {
				name := method + "/" + format.name
				if image == "" {
					name += "/empty-image"
				}
				t.Run(name, func(t *testing.T) {
					var received string
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != method {
							t.Errorf("请求方法 = %s，期望 %s", r.Method, method)
						}
						if method == http.MethodGet {
							received = r.URL.Query().Get("q")
						} else {
							body, err := io.ReadAll(r.Body)
							if err != nil {
								t.Errorf("读取请求体失败: %v", err)
							}
							received = string(body)
						}
						w.WriteHeader(http.StatusNoContent)
					}))
					defer server.Close()

					config := &notification.CustomWebhookChannelConfig{
						Endpoint: server.URL,
						Method:   method,
						Template: format.template,
						Format:   format.name,
					}
					if method == http.MethodGet {
						config.QueryParam = "q"
					}
					handler := NewCustomWebhookChannelHandler(config)
					err := handler.Send(context.Background(), &notification.Notification{
						Image:     image,
						Timestamp: time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC),
					})
					if err != nil {
						t.Fatalf("发送 Webhook 失败: %v", err)
					}

					actualImage, err := format.getImage(received)
					if err != nil {
						t.Fatalf("解析模板结果失败: %v; body=%q", err, received)
					}
					if actualImage != image {
						t.Errorf("模板图片 = %q，期望原始值 %q", actualImage, image)
					}
					if strings.HasPrefix(actualImage, "data:image/") {
						t.Errorf("模板图片不应是 Data URI: %q", actualImage)
					}
				})
			}
		}
	}
}
