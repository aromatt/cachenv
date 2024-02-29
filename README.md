# cash
A memoizing cache for shell commands

## What
This is a CLI tool that provides caching for shell commands. It uses a
virtualenv-like workflow: you create your env, tell it which commands to cache, and
run your commands.

## Features
* Workflow similar to virtualenv
* Command whitelisting by program name, arguments, and patterns
* Streaming input
* File change-detection (for args that are file paths)
* Portable (build your cache on one machine and use it on another)
* Pipelines mixing cached and uncached commands
* Cache entries spanning _across_ pipes

## Use cases
* Building shell scripts involving expensive commands
* Rapidly iterating on shell pipelines (`aws s3 ls ... | grep ... | xargs -n1 awk ...`)
* Testing
* Developing against metered APIs
* Developing against resources with limited availability
