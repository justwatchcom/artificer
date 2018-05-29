# Artificer

## Purpose

Creates docker containers using a base image and the provided files together with a config (environment variables and the run command CMD).
Useful inside of Kubernetes or Docker-containers, as you don't need to access the docker demon.
It then automatically uploads the image to GCR.

## Usage

CLI-Flag | Shorthand | Usage
---|---|---
`--baseimage` | `-b` | The path to the base Image (`FROM` in Dockerfiles)
`--target` | `-t` | The path to the destination repository where the new image gets pushed to.
`--files` | `-f` | The path to a file or directory that will be included at the container root. Use this flag multiple times to specify multiple files or directories.
`--env` | `-e` | An Environment Variable definition of the form `VARIABLE=VALUE`. Use this flag multiple times to specify multiple environment variables
`--cmd` | `-c` | The command that runs automatically once the container is launched (`CMD` in Dockerfiles). 

Example:

```bash
$> artificer \
   --baseimage="eu.gcr.io/some-registry/alpine:latest" \
   --target="eu.gcr.io/some-registry/myprogram:latest" \
   -f "~/go/bin/myprogram" \
   -f "/some/other/file/or/directory" \
   -e "SOMEPATH=/usr/bin/SomePath" \
   -e "SOMEMOREENV=otherEnv" \
   -c "/myprogram --params"
```
       
       
      
