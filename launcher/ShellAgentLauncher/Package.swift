// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "ShellAgentLauncher",
    platforms: [.macOS(.v14)],
    targets: [
        .executableTarget(
            name: "ShellAgentLauncher",
            path: "Sources"
        )
    ]
)
