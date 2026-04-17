import AppKit
import Carbon.HIToolbox

class AppDelegate: NSObject, NSApplicationDelegate {
    private var hotKeyRef: EventHotKeyRef?

    private var mainAppBinary: String {
        // Look for shell-agent binary in standard locations
        let candidates = [
            // Same directory as launcher
            Bundle.main.bundlePath
                .replacingOccurrences(of: "ShellAgentLauncher.app", with: "shell-agent.app")
                + "/Contents/MacOS/shell-agent",
            // Development build (Wails output)
            NSHomeDirectory() + "/works/nlink-jp/_wip/shell-agent/app/build/bin/shell-agent.app/Contents/MacOS/shell-agent",
            // ~/Applications
            NSHomeDirectory() + "/Applications/shell-agent.app/Contents/MacOS/shell-agent",
            // /Applications
            "/Applications/shell-agent.app/Contents/MacOS/shell-agent",
        ]
        for path in candidates {
            if FileManager.default.fileExists(atPath: path) {
                return path
            }
        }
        return candidates[0]
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        registerGlobalHotKey()
    }

    func launchMainApp(action: String?) {
        let binary = mainAppBinary

        guard FileManager.default.fileExists(atPath: binary) else {
            DispatchQueue.main.async {
                let alert = NSAlert()
                alert.messageText = "Shell Agent not found"
                alert.informativeText = "Expected at: \(binary)\nBuild with: cd app && make build"
                alert.alertStyle = .warning
                alert.runModal()
            }
            return
        }

        // Check if already running
        let running = NSWorkspace.shared.runningApplications.first {
            $0.localizedName == "shell-agent" || $0.bundleIdentifier?.contains("shell-agent") == true
        }
        if let app = running {
            app.activate()
            return
        }

        // Launch the binary directly
        let process = Process()
        process.executableURL = URL(fileURLWithPath: binary)
        if let action = action {
            process.arguments = ["--action", action]
        }

        do {
            try process.run()
        } catch {
            DispatchQueue.main.async {
                let alert = NSAlert()
                alert.messageText = "Failed to launch Shell Agent"
                alert.informativeText = error.localizedDescription
                alert.alertStyle = .warning
                alert.runModal()
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

        var eventType = EventTypeSpec(eventClass: OSType(kEventClassKeyboard),
                                       eventKind: UInt32(kEventHotKeyPressed))
        InstallEventHandler(GetApplicationEventTarget(), { _, event, _ -> OSStatus in
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
