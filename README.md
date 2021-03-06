## GitLab CI Multi-purpose Runner

This is GitLab CI Multi-purpose Runner repository an **unofficial GitLab CI runner written in Go**, this application run tests and sends the results to GitLab CI.
[GitLab CI](https://about.gitlab.com/gitlab-ci) is the open-source continuous integration server that coordinates the testing.

This project was made as Go learning opportunity. The initial release was created within two days.

**This is ALPHA. It should work, but also may not.**

[![Build Status](https://travis-ci.org/ayufan/gitlab-ci-multi-runner.svg?branch=master)](https://travis-ci.org/ayufan/gitlab-ci-multi-runner)

### Requirements

**None. This project is designed for the Linux and OS X operating systems.**

### Features

* Allows to run:
 - multiple jobs concurrently
 - use multiple tokens with multiple server (even per-project)
 - limit number of concurrent jobs per-token
* Jobs can be run:
 - locally
 - using Docker container
 - using Docker container and executing job over SSH
 - connecting to remote SSH server
* Is written in Go and distributed as single binary without any other requirements
* Works on Ubuntu, Debian and OS X (should also work on other Linux distributions)
* Allows to customize job running environment
* Automatic configuration reload without restart
* Easy to use setup with support for docker, docker-ssh, parallels or ssh running environments
* Enables caching of Docker containers

### Install and initial configuration

1. Simply download one of this binaries for your system:
	```bash
	sudo wget -O /usr/local/bin/gitlab-ci-multi-runner https://github.com/ayufan/gitlab-ci-multi-runner/releases/download/v0.1.7/gitlab-ci-multi-runner-linux-386
	sudo wget -O /usr/local/bin/gitlab-ci-multi-runner https://github.com/ayufan/gitlab-ci-multi-runner/releases/download/v0.1.7/gitlab-ci-multi-runner-linux-amd64
	sudo wget -O /usr/local/bin/gitlab-ci-multi-runner https://github.com/ayufan/gitlab-ci-multi-runner/releases/download/v0.1.7/gitlab-ci-multi-runner-darwin-386
	sudo wget -O /usr/local/bin/gitlab-ci-multi-runner https://github.com/ayufan/gitlab-ci-multi-runner/releases/download/v0.1.7/gitlab-ci-multi-runner-darwin-amd64
	```

1. Give it permissions to execute:
	```bash
	sudo chmod +x /usr/local/bin/gitlab-ci-multi-runner
	```

1. If you want to use Docker - install Docker:
    ```bash
    curl -sSL https://get.docker.com/ | sh
    ```

1. Create a GitLab CI user (Linux)
	```
	sudo adduser --disabled-login --gecos 'GitLab Runner' gitlab_ci_runner
	sudo usermod -aG docker gitlab_ci_runner
	sudo su gitlab_ci_runner
	cd ~/
	```

1. Setup the runner
	```bash
	$ gitlab-ci-multi-runner-linux setup
	Please enter the gitlab-ci coordinator URL (e.g. http://gitlab-ci.org:3000/ )
	https://ci.gitlab.org/
	Please enter the gitlab-ci token for this runner
	xxx
	Please enter the gitlab-ci description for this runner
	my-runner
	INFO[0034] fcf5c619 Registering runner... succeeded
	Please enter the executor: shell, docker, docker-ssh, ssh?
	docker
	Please enter the Docker image (eg. ruby:2.1):
	ruby:2.1
	INFO[0037] Runner registered successfully. Feel free to start it, but if it's running already the config should be automatically reloaded!
	```

	* Definition of hostname will be available with version 7.8.0 of GitLab CI.

1. Run the runner
	```bash
	$ screen
	$ gitlab-ci-multi-runner run
	```

1. Add to cron
	```bash
	$ crontab -e
	@reboot gitlab-ci-multi-runner run &>log/gitlab-ci-multi-runner.log
	```

### Extra projects?

If you want to add another project, token or image simply re-run setup. *You don't have to re-run the runner. He will automatically reload configuration once it changes.*

### Config file

Configuration uses TOML format described here: https://github.com/toml-lang/toml

1. The global section:
    ```
    concurrent = 4
    root_dir = ""
    ```
    
    This defines global settings of multi-runner:
    * `concurrent` - limits how many jobs globally can be run concurrently. The most upper limit of jobs using all defined runners
    * `root_dir` - allows to change relative dir where all builds, caches, etc. are stored. By default is current working directory

1. The [[runners]] section:
    ```
    [[runners]]
      name = "ruby-2.1-docker"
      url = "https://CI/"
      token = "TOKEN"
      limit = 0
      executor = "docker"
      builds_dir = ""
      shell_script = ""
      clean_environment = false
      environment = ["ENV=value", "LC_ALL=en_US.UTF-8"]
    ```

    This defines one runner entry:
    * `name` - not used, just informatory
    * `url` - CI URL
    * `token` - runner token
    * `limit` - limit how many jobs can be handled concurrently by this token. 0 simply means don't limit.
    * `executor` - select how project should be built. See below.
    * `builds_dir` - directory where builds will be stored in context of selected executor (Locally, Docker, SSH)
    * `clean_environment` - do not inherit any environment variables from the multi-runner process
    * `environment` - append or overwrite environment variables

1. The EXECUTORS:

    There are a couple of available executors currently:
    * **shell** - run build locally, default
    * **docker** - run build using Docker container - this requires the presence of *[runners.docker]*
    * **docker-ssh** - run build using Docker container, but connect to it with SSH - this requires the presence of *[runners.docker]* and *[runners.ssh]*
    * **ssh** - run build remotely with SSH - this requires the presence of *[runners.ssh]*
    * **parallels** - run build using Parallels VM, but connect to it with SSH - this requires the presence of *[runners.parallels]* and *[runners.ssh]*

1. The [runners.docker] section:
    ```
    [runners.docker]
      host = ""
      hostname = ""
      image = "ruby:2.1"
      privileged = false
      disable_cache = false
      disable_pull = false
      cache_dir = ""
      registry = ""
      volumes = ["/data", "/home/project/cache"]
      extra_hosts = ["other-host:127.0.0.1"]
      links = ["mysql_container:mysql"]
      services = ["mysql", "redis:2.8", "postgres:9"]
    ```
    
    This defines the Docker Container parameters:
    * `host` - specify custom Docker endpoint, by default *DOCKER_HOST* environment is used or *"unix:///var/run/docker.sock"*
    * `hostname` - specify custom hostname for Docker container
    * `image` - use this image to run builds
    * `privileged` - make container run in Privileged mode (insecure)
    * `disable_cache` - disable automatic
    * `disable_pull` - disable automatic image pulling if not found
    * `cache_dir` - specify where Docker caches should be stored (this can be absolute or relative to current working directory)
    * `registry` - specify custom Docker registry to be used
    * `volumes` - specify additional volumes that should be cached
    * `extra_hosts` - specify hosts that should be defined in container environment
    * `links` - specify containers which should be linked with building container
    * `services` - specify additional services that should be run with build. Please visit [Docker Registry](https://registry.hub.docker.com/) for list of available applications. Each service will be run in separate container and linked to the build.

1. The [runners.parallels] section:
    ```
    [runners.parallels]
      base_name = "my-parallels-image"
      template_name = ""
      disable_snapshots = false
    ```

    This defines the Parallels parameters:
    * `base_name` - name of Parallels VM which will be cloned
    * `template_name` - custom name of Parallels VM linked template (optional)
    * `disable_snapshots` - if disabled the VMs will be destroyed after build

1. The [runners.ssh] section:
    ```
    [runners.ssh]
      host = "my-production-server"
      port = "22"
      user = "root"
      password = "production-server-password"
    ```
    
    This defines the SSH connection parameters:
    * `host` - where to connect (it's override when using *docker-ssh*)
    * `port` - specify port, default: 22
    * `user` - specify user
    * `password` - specify password

1. Example configuration file
    [Example configuration file](config.toml.example)

## Example integrations

### How to configure runner for GitLab CI integration tests?

1. Run setup
    ```bash
    $ gitlab-ci-multi-runner-linux setup
    Please enter the gitlab-ci coordinator URL (e.g. http://gitlab-ci.org:3000/ )
    https://ci.gitlab.com/
    Please enter the gitlab-ci token for this runner
    REGISTRATION_TOKEN
    Please enter the gitlab-ci description for this runner
    my-gitlab-ci-runner
    INFO[0047] 5bf4aa89 Registering runner... succeeded
    Please enter the executor: shell, docker, docker-ssh, ssh?
    docker
    Please enter the Docker image (eg. ruby:2.1):
    ruby:2.1
    If you want to enable mysql please enter version (X.Y) or enter latest?
    latest
    If you want to enable postgres please enter version (X.Y) or enter latest?
    latest
    If you want to enable redis please enter version (X.Y) or enter latest?
    latest
    If you want to enable mongodb please enter version (X.Y) or enter latest?

    INFO[0069] Runner registered successfully. Feel free to start it, but if it's running already the config should be automatically reloaded!
    ```

1. Run the multi-runner if you didn't already
    ```bash
    $ gitlab-ci-multi-runner-linux run
    ```

1. Add job to test with MySQL
    ```bash
    wget -q http://ftp.de.debian.org/debian/pool/main/p/phantomjs/phantomjs_1.9.0-1+b1_amd64.deb
    dpkg -i phantomjs_1.9.0-1+b1_amd64.deb

    bundle install --deployment --path /cache

    cp config/application.yml.example config/application.yml

    cp config/database.yml.mysql config/database.yml
    sed -i 's/username:.*/username: root/g' config/database.yml
    sed -i 's/password:.*/password:/g' config/database.yml
    sed -i 's/# socket:.*/host: mysql/g' config/database.yml

    cp config/resque.yml.example config/resque.yml
    sed -i 's/localhost/redis/g' config/resque.yml

    bundle exec rake db:create
    bundle exec rake db:setup
    bundle exec rake spec
    ```

1. Add job to test with PostgreSQL
    ```bash
    wget -q http://ftp.de.debian.org/debian/pool/main/p/phantomjs/phantomjs_1.9.0-1+b1_amd64.deb
    dpkg -i phantomjs_1.9.0-1+b1_amd64.deb

    bundle install --deployment --path /cache

    cp config/application.yml.example config/application.yml

    cp config/database.yml.postgresql config/database.yml
    sed -i 's/username:.*/username: postgres/g' config/database.yml
    sed -i 's/password:.*/password:/g' config/database.yml
    sed -i 's/# socket:.*/host: postgres/g' config/database.yml

    cp config/resque.yml.example config/resque.yml
    sed -i 's/localhost/redis/g' config/resque.yml

    bundle exec rake db:create
    bundle exec rake db:setup
    bundle exec rake spec
    ```

1. Voila! You now have GitLab CI integration testing instance with bundle caching. Push some commits to test it.

1. Look into `config.toml` and tune it.

### FAQ

TBD

### Future

* It should be simple to add additional executors: DigitalOcean? Amazon EC2?
* Tests!

### Author

[Kamil Trzciński](mailto:ayufan@ayufan.eu), 2015, [Polidea](http://www.polidea.com/)

### License

GPLv3
