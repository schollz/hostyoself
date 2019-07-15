var files = [];
var isConnected = false;
var relativeDirectory = "";

function consoleLog(s) {
    console.log(s);
    if (typeof s === 'object') {
        s = JSON.stringify(s);
    }

    if (!(s.startsWith("[debug]"))) {
        document.getElementById("consoleText").value = document.getElementById("consoleText").value + s + "\n";
        document.getElementById("consoleText").scrollTop = document.getElementById("consoleText").scrollHeight;
    }
}

function humanFileSize(bytes, si) {
    var thresh = si ? 1000 : 1024;
    if (Math.abs(bytes) < thresh) {
        return bytes + ' B';
    }
    var units = si ? ['kB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'] : ['KiB', 'MiB', 'GiB', 'TiB', 'PiB',
        'EiB', 'ZiB', 'YiB'
    ];
    var u = -1;
    do {
        bytes /= thresh;
        ++u;
    } while (Math.abs(bytes) >= thresh && u < units.length - 1);
    return bytes.toFixed(1) + ' ' + units[u];
}

var Name = "";
var filesize = 0;

(function(Dropzone) {
    Dropzone.autoDiscover = false;

    let drop = new Dropzone('div#filesBox', {
        maxFiles: 1000,
        url: '/',
        method: 'post',
        createImageThumbnails: false,
        previewTemplate: "<div id='preview' class='.dropzone-previews'></div>",
        autoProcessQueue: false,
    });


    drop.on('addedfile', function(file) {
        // console.log(file);
        var domain = document.getElementById("inputDomain").value
        files.push(file);
        if (file.hasOwnProperty("webkitRelativePath")) {
            if (files.length == 1) {
                relativeDirectory = file.webkitRelativePath.split("/")[0];
            } else if (file.webkitRelativePath.split("/")[0] != relativeDirectory) {
                relativeDirectory = "";
            }
        }


        if (!(isConnected)) {
            isConnected = true;
            socketSend({
                type: "domain",
                message: domain,
                key: document.getElementById("inputKey").value,
            })
        }

        var filesString = "files are";
        var domainName = `${window.publicURL}/${domain}/`;
        if (files.length == 1) {
            filesString = "file is"
            domainName += `${file.name}`
        }

        document.getElementById("consoleHeader").innerHTML =
            `<p>Your ${filesString} available at:<br> <center><strong><a href="${domainName}" target="_blank">${domainName}</a></strong></center></p>`;
        html = `<ul>`
        for (i = 0; i < files.length; i++) {
            var urlToFile = files[i].name;
            if ('fullPath' in files[i]) {
                urlToFile = files[i].fullPath;
            }
            html = html +
                `<li><a href="/${domain}/${urlToFile}" target="_blank">/${urlToFile}</a></li>`
        }
        html = html + `</ul>`;
        document.getElementById("fileList").innerHTML = html;
        document.getElementById("filesBox").classList.add("hide");
        document.getElementById("console").classList.remove("hide");
        document.getElementById("inputKey").readOnly = "true";
        document.getElementById("inputDomain").readOnly = "true";
    })

})(Dropzone);

var socket; // websocket


/* websockets */
function socketSend(data) {
    if (socket == null) {
        return
    }
    if (socket.readyState != 1) {
        return
    }
    jsonData = JSON.stringify(data);
    socket.send(jsonData);
    if (jsonData.length > 100) {
        consoleLog("[debug] ws-> " + jsonData.substring(0, 99))
    } else {
        consoleLog("[debug] ws-> " + jsonData)
    }
}

const socketMessageListener = (event) => {
    var data = JSON.parse(event.data);
    if (!('type' in data && 'message' in data)) {
        consoleLog(`[warn] got bad data ${event.data}`);
        return
    }
    console.log(data)
    consoleLog(`[debug] ${data.message}`)
    if (data.type == "files") {
        if (files.length > 0) {
            socketSend({
                type: "files",
                message: JSON.stringify(files),
                success: true,
                key: document.getElementById("inputKey").value,
            });
            consoleLog(
                `${data.ip} [${(new Date()).toUTCString()}] sitemap 200`
            );
        } else {
            socketSend({
                type: "files",
                message: "none found",
                success: false,
                key: document.getElementById("inputKey").value,
            });
            consoleLog(
                `${data.ip} [${(new Date()).toUTCString()}] sitemap 404`
            );
        }
    } else if (data.type == "get") {
        var foundFile = false
        var iToSend = 0
        for (i = 0; i < files.length; i++) {
            if (files[i].webkitRelativePath == data.message || files[i].fullPath == data.message || files[i].name == data.message || files[i]
                .webkitRelativePath == relativeDirectory + "/" + data.message || files[i]
                .fullPath == relativeDirectory + "/" + data.message) {
                iToSend = i;
                var reader = new FileReader();
                reader.onload = function(theFile) {
                    socketSend({
                        type: "get",
                        message: reader.result,
                        success: true,
                        key: document.getElementById("inputKey").value,
                    })
                    consoleLog(
                        `${data.ip} [${(new Date()).toUTCString()}] /${data.message} 200 ${files[i].size}`
                    );
                };
                reader.readAsDataURL(files[i]);
                foundFile = true
                break
            }
        }
        if (foundFile == false) {
            socketSend({
                type: "get",
                message: "not found",
                success: false,
                key: document.getElementById("inputKey").value,
            })
            consoleLog(`${data.ip} [${(new Date()).toUTCString()}] /${data.message} 404`);
        }
    } else if (data.type == "domain") {
        console.log(`[info] ${data.message}`);
    } else if (data.type == "message") {
        console.log(`[info] ${data.message}`);
    } else {
        consoleLog(`[debug] unknown`);
    }
};
const socketOpenListener = (event) => {
    consoleLog('[info] connected');
    if (isConnected == true) {
        // reconnect if was connected and got disconnected
        socketSend({
            type: "domain",
            message: document.getElementById("inputDomain").value,
            key: document.getElementById("inputKey").value,
        })
    }
};

const socketCloseListener = (event) => {
    if (socket) {
        consoleLog('[info] disconnected');
    }
    var url = window.origin.replace("http", "ws") + '/ws';
    try {
        socket = new WebSocket(url);
        socket.addEventListener('open', socketOpenListener);
        socket.addEventListener('message', socketMessageListener);
        socket.addEventListener('close', socketCloseListener);
    } catch (err) {
        consoleLog("[info] no connection available")
    }
};


socketCloseListener();