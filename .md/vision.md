Project Vision: The Native Player Transition

1. Executive Summary

We are transitioning the current digital signage player from a Chromium/Node.js stack to a Native Go + libVLC architecture. This shift is designed to solve performance bottlenecks on the Raspberry Pi 4 and fully leverage the 64-bit architecture and hardware acceleration capabilities of the Raspberry Pi 5.

2. Problem Statement

The current implementation relies on a "headless" browser (Chromium) and Puppeteer. While flexible, this approach suffers from:

High Resource Overhead: Chromium consumes significant RAM (~800MB+) and CPU cycles.

Decoding Inefficiency: Browsers often struggle with hardware acceleration for 4K HEVC streams, leading to dropped frames and lag.

Process Complexity: Managing Node.js, PM2, and a browser instance creates multiple points of failure.

3. The New Paradigm: "Player-Native"

By moving to a native Go application, we achieve a "Zero-Intervention" failsafe system that is purpose-built for video.

A. Technical Pillars

Pillar

Strategy

Benefit

Direct Hardware Access

Using libVLC with mmal_vout

Bypasses the desktop compositor for smooth 4K/60fps playback.

Compiled Execution

Single Go Binary

Reduces startup time and eliminates "Dependency Hell" (node_modules).

Real-time Management

fsnotify Folder Watching

Allows for instant playlist updates without restarting the application.

OS Integration

Native .deb & Systemd

Makes the player a first-class citizen of the Linux OS.

4. Operational Roadmap

Phase 1: Core Engine Development

Implement the Go/VLC bridge.

Stress-test 4K HEVC playback using DRM/KMS.

Establish the folder-watching logic for the /playlist directory.

Phase 2: Integration & Porting

Port the legacy config.json identity management from the Node.js project.

Implement the API heartbeat service for remote monitoring.

Develop the "Maintenance Service" for disk and thermal management.

Phase 3: Deployment Engineering

Create the multi-stage Dockerfile for containerized environments.

Build the Debian packaging pipeline for sudo apt install support.

Finalize the Systemd unit for automatic recovery on boot or crash.

5. Success Metrics

Performance: Steady 60fps on 4K content with zero frame drops.

Memory Footprint: Application RAM usage stays below 100MB (excluding video buffer).

Stability: 99.9% uptime without the need for manual PM2 resets.

Deployment Speed: System setup reduced from multiple manual steps to a single command installation.