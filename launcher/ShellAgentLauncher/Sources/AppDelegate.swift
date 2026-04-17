import AppKit
import Carbon.HIToolbox

class AppDelegate: NSObject, NSApplicationDelegate {
    private var hotKeyRef: EventHotKeyRef?
    private var mainAppPath: String {
        // Look for shell-agent.app in standard locations
        let candidates = [
            // Same directory as launcher
            Bundle.main.bundlePath
                .replacingOccurrences(of: "ShellAgentLauncher.app", with: "shell-agent.app"),
            // ~/Applications
            NSHomeDirectory() + "/Applications/shell-agent.app",
            // /Applications
            "/Applications/shell-agent.app",
            // Development build
            NSHomeDirectory() + "/works/nlink-jp/_wip/shell-agent/app/build/bin/shell-agent.app",
        ]
        for path in candidates {
            if FileManager.default.fileExists(atPath: path) {
                return path
            }
        }
        return candidates[0]
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Hide dock icon (menu bar only)
        NSApp.setActivationPolicy(.accessory)

        // Register global hotkey: Ctrl+Shift+Space
        registerGlobalHotKey()
    }

    func launchMainApp(action: String?) {
        let appURL = URL(fileURLWithPath: mainAppPath)

        let config = NSWorkspace.OpenConfiguration()
        config.activates = true

        if let action = action {
            config.arguments = ["--action", action]
        }

        NSWorkspace.shared.openApplication(at: appURL, configuration: config) { app, error in
            if let error = error {
                DispatchQueue.main.async {
                    let alert = NSAlert()
                    alert.messageText = "Failed to launch Shell Agent"
                    alert.informativeText = error.localizedDescription
                    alert.alertStyle = .warning
                    alert.runModal()
                }
            }
        }
    }

    // MARK: - Global Hot Key (Ctrl+Shift+Space)

    private func registerGlobalHotKey() {
        let hotKeyID = EventHotKeyID(signature: OSType(0x5341474E), // "SAGN"
                                      id: 1)
        let modifiers: UInt32 = UInt32(controlKey | shiftKey)
        let keyCode: UInt32 = UInt32(kVK_Space)

        var ref: EventHotKeyRef?
        let status = RegisterEventHotKey(keyCode, modifiers, hotKeyID,
                                          GetApplicationEventTarget(), 0, &ref)
        if status == noErr {
            hotKeyRef = ref
        }

        // Install event handler
        var eventType = EventTypeSpec(eventClass: OSType(kEventClassKeyboard),
                                       eventKind: UInt32(kEventHotKeyPressed))
        InstallEventHandler(GetApplicationEventTarget(), { _, event, _ -> OSStatus in
            // Trigger launch on hotkey press
            DispatchQueue.main.async {
                if let delegate = NSApp.delegate as? AppDelegate {
                    delegate.launchMainApp(action: nil)
                }
            }
            return noErr
        }, 1, &eventType, nil, nil)
    }

    func applicationWillTerminate(_ notification: Notification) {
        if let ref = hotKeyRef {
            UnregisterEventHotKey(ref)
        }
    }
}
