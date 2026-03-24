//go:build windows

// internal/client/notify/notify_windows.go
package notify

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// WindowsNotifier implements the Notifier interface for Windows using
// PowerShell to invoke the Windows Toast notification API.
//
// Click detection: Toast notifications include an activationType="protocol"
// action with a trasker:// URL. The client must register a trasker:// protocol
// handler in the Windows registry (HKCU\Software\Classes\trasker\...) that
// routes back to the running process via a localhost HTTP endpoint.
// The webui HTTP server (Plan 05) handles GET /deadman-ack/:id → presence.Acknowledge().
type WindowsNotifier struct {
	appID   string
	clickMu sync.Mutex
	clickFn ClickAction
	counter uint64
}

// NewWindowsNotifier creates a Windows notifier.
func NewWindowsNotifier() (*WindowsNotifier, error) {
	return &WindowsNotifier{
		appID: "Trasker",
	}, nil
}

// Notify displays a Windows Toast notification via PowerShell.
// onClick is called if the user clicks the notification (may be nil).
//
// Note: Click callbacks on Windows require the trasker:// protocol handler
// to be registered and the local HTTP server to be running. The onClick
// function is stored but invocation depends on the protocol handler routing.
func (n *WindowsNotifier) Notify(title, body string, onClick ClickAction) error {
	n.clickMu.Lock()
	n.counter++
	id := fmt.Sprintf("trasker-%d", n.counter)
	if onClick != nil {
		n.clickFn = onClick
	}
	n.clickMu.Unlock()

	// Escape single quotes for PowerShell strings.
	safeTitle := strings.ReplaceAll(title, "'", "''")
	safeBody := strings.ReplaceAll(body, "'", "''")
	safeID := strings.ReplaceAll(id, "'", "''")
	safeAppID := strings.ReplaceAll(n.appID, "'", "''")

	// Build the PowerShell script for a toast notification.
	script := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$template = @"
<toast>
    <visual>
        <binding template="ToastGeneric">
            <text>%s</text>
            <text>%s</text>
        </binding>
    </visual>
    <actions>
        <action content="I'm here" arguments="trasker://deadman-ack/%s" activationType="protocol"/>
    </actions>
</toast>
"@

$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($template)
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('%s').Show($toast)
`, safeTitle, safeBody, safeID, safeAppID)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("notify windows: toast notification failed: %w: %s", err, string(output))
	}
	return nil
}

// Close is a no-op for the Windows notifier (no persistent resources).
func (n *WindowsNotifier) Close() error {
	return nil
}
