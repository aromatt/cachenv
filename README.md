# cash
A versatile memoizing cache for program invocations, with a virtualenv-like workflow.

## Overview
`cash` is a tool that optimizes the execution of programs by caching their
outputs, including stdout, stderr, and exit codes. It uses arguments and (optionally)
stdin as cache keys.

It's like function memoization, but at the process boundary.

This is useful in a variety of development tasks, especially testing and rapid
iteration.

## Features
* **Comprehensive Caching**: Captures stdout, stderr, and exit codes, providing a complete snapshot of a program's behavior given a particular input.
* **Faithful Replication**: Interleaves stdout and stderr in the cache, mirroring the original program's behavior.
* **Selective Memoization**: Supports precise configuration to selectively cache based on program name, arguments, and/or input patterns.
* **File Awareness**: Sensitive to changes in the contents of files provided as arguments.
* **Pipeline Compatibility**: Naturally integrates with command pipelines, allowing a mix of cached and live executions within complex command chains. Also includes optimizations which can effectively cache entire pipelines.
* **Streaming Support**: Includes a streaming mode for line-oriented programs.
* **Cross-Environment Portability**: Enables cache sharing and reuse across different machines and operating systems.
* **Cache Management**: Offers configurable entry management, including size limits and eviction policies.

## Use Cases
* **Optimizing Development Workflows**: Accelerates development and testing cycles for software that relies on external programs or data processing commands.
* **Improving Script Performance**: Boost the performance of scripts by caching the results of expensive program executions, making repeated invocations significantly faster.
* **Efficient Resource Utilization**: Reduces calls to metered APIs and conserves bandwidth by caching program interactions that would otherwise redundantly fetch the same data.
* **Reliable Testing Environments**: Ensures consistent and quick retrieval of program outputs for testing frameworks and automated scripts, minimizaing external dependencies.
