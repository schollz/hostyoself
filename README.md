<p align="center">
<img
    src="/static/banner.jpg" border="0" alt="hostyoself">
<br>
<a
href="https://github.com/schollz/hostyoself/releases/latest"><img
src="https://img.shields.io/badge/version-0.0.1-brightgreen.svg?style=flat-square"
alt="Version"></a> </p>

<p align="center">A hosting service from the browser, because why not. Try it at <a href="https://hostyoself.com">hostyoself.com</a>.</p>


## Host from the browser

Open [hostyoself.com](https://hostyoself.com) and drag and drop a folder, or select a file. Your browser will host the files!

## Host from the command line

You can host files directly from the terminal!

```
$ hostyoself host
https://hostyoself.com/confidentcat/
```

Now if you have a file in your folder `README.md` you can access it with the public URL `https://hostyoself.com/confidentcat/README.md`, directly from your computer!


## Run your own relay

Want to run your own relay? Its easy. 

```
$ hostyoself relay --url https://yoururl
```

## FAQ


**How do I start web hosting?** You will need to setup port forwarding, a dynamic DNS, name registration, MySQL, PHP, Apache and take a online course in Javascript. 

Just *kidding*! You don't need any of that crap. Just goto [hostyoself.com](https://hostyoself.com) drag and drop a folder, or select a file. That's literally it. Now you can host a website from your laptop or your phone or your smartwatch or your toaster.

**How is this possible?** When the server you point at gets a request for a webpage, the server turns back and asks *you* for that content and will use what you provide for the original request.

**Seriously, how is this possible?** The relay uses websockets in your browser to process GET commands.

**Won't my website disappear when I close my browser?** Yep! There is a [command-line tool](https://github.com/schollz/hostyoself#host-from-the-command-line) that doesn't require a browser so it can run in the background if you need that. But yes, if your computer turns off then your site is down. Welcome to the joys of hosting a site on the internet.

**Won't I have to reload my browser if I change a file?** Yep! Welcome to the joys of Javascript.

**Whats the largest file I can host using this?** `¯\_(ツ)_/¯`

**Should I use this to host a website?** Dear god yes.

**Does this use AI or blockchain?** Sure, why not. 

**What inspired this?** [beaker browser](https://beakerbrowser.com/), [ngrok](https://ngrok.com/), [localhost.run](http://localhost.run/), [inlets.dev](https://github.com/alexellis/inlets), Parks and Recreation.

**What's the point of this?** You can host a website! You can share a file! Anything you want, directly from your browser!

## Develop

```
$ git clone https://github.com/schollz/hostyoself
$ cd hostyoself
$ go generate -v -x
$ go build -v
```

## License 

MIT