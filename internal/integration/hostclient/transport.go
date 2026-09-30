// Ограниченный read-only HTTP обмен с локальным Host; не принимает произвольные маршруты.
package hostclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
)

// readDevice получает только JSON-снимок /api/device у запустившего приложение Host.
// Не следует redirect, ограничивает тело 64 KiB и отвергает лишние JSON-документы.
func (c *Client) readDevice(ctx context.Context, target *device) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.address+"/api/device", nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-store")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK || err != nil || mediaType != "application/json" {
		return fmt.Errorf("invalid Host response")
	}
	const maxBytes = 64 << 10
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil || len(body) > maxBytes {
		return fmt.Errorf("Host response exceeds limit or cannot be read")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("invalid Host JSON: %w", err)
	}
	return nil
}
