# cachenv
Versatile memoizing cache for program invocations, with a `virtualenv`-like
interface.

Note: this repo is still in early development; the README is aspirational.

## Overview
`cachenv` is a lightweight tool that provides caching for your commands,
scripts, and pipelines. In fact, any program that calls `exec()` can use `cachenv`.

It's like function [memoization](https://en.wikipedia.org/wiki/Memoization), but at
the process boundary. This is useful in a variety of contexts, especially testing and
rapid iteration.

The workflow mirrors that of [virtualenv](https://virtualenv.pypa.io/en/latest/) -
you create an environment, activate it, and work within it.

## Use Cases
* Creating consistent, dependency-free testing environments
* Rapidly iterating on scripts or notebooks that rely on external services or data processing
* Efficiently constructing CLI pipelines that involve large inputs or expensive filters/aggregations
* Eliminating redundant calls to metered APIs

## Usage
Initialize and activate a new cachenv:
```
$ cachenv init .cachenv
Created activate script at .cachenv/activate

$ source .cachenv/activate
```

Enable memoization for `ls`:
```
(.cachenv) $ cachenv add ls
Command 'ls' added to memoized commands.
Refreshed symlink for ls
```

Enjoy(?) memoization for `ls`:
```
(.cachenv) $ ls
foo

(.cachenv) $ touch bar
(.cachenv) $ ls
foo

```

Try diff mode:
```
(.cachenv) $ cachenv diff ls
0a1
> bar
```

## Features
<table>
  <tr>
    <th>Implemented</th>
    <th>Feature</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>✅</td>
    <td><strong>Comprehensive Caching</strong></td>
    <td>Captures stdout, stderr, and exit codes, providing a complete snapshot of a
        program's behavior given a particular input.</td>
  </tr>
  <tr>
    <td></td>
    <td><strong>Selective Memoization</strong></td>
    <td>Supports precise configuration to selectively enable caching based on program
        name, arguments, and/or input patterns.</td>
  </tr>
  <tr>
    <td></td>
    <td><strong>Streaming Mode</strong></td>
    <td>Supports caching at the line level, keyed by stdin.</td>
  </tr>
  <tr>
    <td></td>
    <td><strong>File Awareness</strong></td>
    <td>Can optionally distinguish cache entries based on the contents of files
        provided as arguments (e.g., for <code>grep foo bar.txt</code>, refresh
        the cache when the content of <code>bar.txt</code> changes).</td>
  </tr>
  <tr>
    <td></td>
    <td><strong>Pipeline Compatibility</strong></td>
    <td>Naturally integrates with command pipelines, allowing a mix of cached and
        live executions within complex command chains. Also includes
        optimizations which can effectively cache entire pipelines.</td>
  </tr>
  <tr>
    <td>✅</td>
    <td><strong>Diff Mode</strong></td>
    <td>Can show changes in a program's behavior against a cached snapshot.</td>
  </tr>
  <tr>
    <td>✅</td>
    <td><strong>Cross-Environment Portability</strong></td>
    <td>Enables cache sharing and reuse across different machines and operating
        systems.</td>
  </tr>
</table>
