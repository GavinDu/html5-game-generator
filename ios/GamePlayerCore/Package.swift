// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "GamePlayerCore",
    platforms: [.iOS(.v16), .macOS(.v13)],
    products: [
        .library(name: "GamePlayerCore", targets: ["GamePlayerCore"])
    ],
    targets: [
        .target(name: "GamePlayerCore"),
        .testTarget(name: "GamePlayerCoreTests", dependencies: ["GamePlayerCore"])
    ]
)
