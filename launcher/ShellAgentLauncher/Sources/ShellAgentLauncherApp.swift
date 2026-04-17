import SwiftUI
import Carbon.HIToolbox

@main
struct ShellAgentLauncherApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var appDelegate

    var body: some Scene {
        MenuBarExtra("Shell Agent", systemImage: "terminal.fill") {
            Button("New Chat") {
                appDelegate.launchMainApp(action: "new")
            }
            .keyboardShortcut("n", modifiers: [.command])

            Divider()

            Button("Open Shell Agent") {
                appDelegate.launchMainApp(action: nil)
            }
            .keyboardShortcut("o", modifiers: [.command])

            Divider()

            Button("Settings...") {
                appDelegate.launchMainApp(action: "settings")
            }
            .keyboardShortcut(",", modifiers: [.command])

            Divider()

            Button("Quit") {
                NSApplication.shared.terminate(nil)
            }
            .keyboardShortcut("q", modifiers: [.command])
        }
    }
}
