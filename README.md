# cachenv
Versatile memoizing cache for program invocations, with a `virtualenv`-like
interface.

## Overview
`cachenv` is a lightweight tool that provides caching for your commands,
scripts, and pipelines. In fact, any program that calls `exec()` can use `cachenv`.

It's like function [memoization](https://en.wikipedia.org/wiki/Memoization), but at
the process boundary. This is useful in a variety of contexts, especially testing and
rapid iteration.

The workflow mirrors that of [virtualenv](https://virtualenv.pypa.io/en/latest/) -
you create an environment, activate it, and work within it.

## Features
<table>
  <tr>
    <td><strong>Comprehensive Caching</strong></td>
    <td>Captures stdout, stderr, and exit codes, providing a complete snapshot of a
        program's behavior given a particular input.</td>
  </tr>
  <tr>
    <td><strong>Selective Memoization</strong></td>
    <td>Supports precise configuration to selectively enable caching based on program
        name, arguments, and/or input patterns.</td>
  </tr>
  <tr>
    <td><strong>Streaming Mode</strong></td>
    <td>Supports caching at the line level, keyed by stdin.</td>
  </tr>
  <tr>
    <td><strong>File Awareness</strong></td>
    <td>Can optionally distinguish cache entries based on the contents of files
        provided as arguments (e.g., for <code>grep foo bar.txt</code>, refresh
        the cache when the content of <code>bar.txt</code> changes).</td>
  </tr>
  <tr>
    <td><strong>Pipeline Compatibility</strong></td>
    <td>Naturally integrates with command pipelines, allowing a mix of cached and
        live executions within complex command chains. Also includes
        optimizations which can effectively cache entire pipelines.</td>
  </tr>
  <tr>
    <td><strong>Diff Mode</strong></td>
    <td>Can show changes in a program's behavior against a cached snapshot.</td>
  </tr>
  <tr>
    <td><strong>Faithful Replication</strong></td>
    <td>Interleaves lines of stdout and stderr in the same order they were generated
        by the original program (when possible).</td>
  </tr>
  <tr>
    <td><strong>Cross-Environment Portability</strong></td>
    <td>Enables cache sharing and reuse across different machines and operating
        systems.</td>
  </tr>
</table>

## Use Cases

* **Optimizing Development Workflows**: Accelerates development and testing cycles for software that relies on external programs or data processing commands.
* **Improving Script Performance**: Boost the performance of scripts (or compiled programs) by caching the results of expensive program executions, making repeated invocations significantly faster.
* **Efficient Resource Utilization**: Reduces calls to metered APIs and conserves bandwidth by caching program interactions that would otherwise redundantly fetch the same data.
* **Reliable Testing Environments**: Ensures consistent and quick retrieval of program outputs for testing frameworks and automation, minimizaing external dependencies.
