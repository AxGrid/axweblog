$(function () {
    //const
    const serviceKey = ["time", "level", "message"]
    const types = {
        "warn": "wrn",
        "debug": "dbg",
        "info": "inf",
        "error": "err"
    }


    //vars
    let html = '';
    let data = [];
    let Follow = true;
    let typeLevel = Cookies.get('level') ? Cookies.get('level') : "debug";
    let lineBreak = Cookies.get('lineBreak');
    let uuid = 0;
    let MaxLogs = 2000;
    let global_id = 0;
    let request_id = 0;
    let finder;

    //func
    const typeSwitcher = (message) => {
        switch (typeof message) {
            case "object":
                return JSON.stringify(message)
            case "string":
                return `"${message}"`
            default:
                return message
        }
    }
    const dateFormat = (data) => {
        const addZero = (value) => {
            if (value > 9) return value
            return `0${value}`
        }
        return `${addZero(data.getHours())}:${addZero(data.getMinutes())}:${addZero(data.getSeconds())}`
    }
    const fetchData = () => {
        let uniqQuery = uuid ? `&uniq=${uuid}` : ""
        let func_request_id = ++global_id;
        $.ajax({
            url: `./lp/?r=${request_id}${uniqQuery}`,
            dataType: 'json',
            timeout: timeout
        }).then((data, status, response) => {
            if (func_request_id !== global_id) return;
            uuid = response.getResponseHeader("Uniq")
            request_id = data[data.length - 1].ID
            writeData(data);
            fetchData()
        }).catch(e => {
            setTimeout(() => {
                fetchData()
            }, 1000)
        })
    }

    const writeData = (elements) => {
        html = ''
        data = data.concat(elements)
        data = [...new Map(data.map(item => [item["ID"], item])).values()];
        sortData()

        if (data.length > MaxLogs){
            data = data.slice(data.length - MaxLogs, data.length - 1)
        }

        $.each(data, function (key, value) {
            html += `<div class="item ${value.Data.level}">`;
            //for debug
            html += `<span class="id me-3">${getID(value.ID)}</span>`
            //end
            html += `<span class="time">${dateFormat(new Date(value.Data.time))} </span>`;
            html += `<span class="type">${types[value.Data.level]} </span>`;
            html += `<span class="message">${value.Data.message} </span>`;
            html += `<span class="err">error=${typeSwitcher(value.Data.error)} </span>`;
            Object.keys(value.Data).map(key => {
                if (!serviceKey.includes(key)) {
                    html += ` <div class="var"><span class="title">${key}=</span><span>${typeSwitcher(value.Data[key])}</span></div> `
                }
            })
            html += '</div>';
        });
        $('#container').html(html);
        if (data.length < MaxLogs && Follow) {
            scrollToBottom();
        }
        searchVal()
    }

    const sortData = () => {
        let arr
        switch (typeLevel) {
            case "info":
                arr = ["info", "warn", "error"]
                data = data.filter(e => arr.includes(e.Data.level))
                break
            case "warn":
                arr = ["warn", "error"]
                data = data.filter(e => arr.includes(e.Data.level))
                break
            case "error":
                data = data.filter(e => e.Data.level === "error")
                break
            default:
                break
        }
    }

    const scrollToBottom = () => {

        /*$("#container").animate({
                scrollTop: (Number.MAX_SAFE_INTEGER),
                duration: 1
            });*/
        $(window).scrollTop(Number.MAX_SAFE_INTEGER)
        //$('#container').scrollTop = -$('#container').scrollHeight
        //window.scrollTo(0, document.body.scrollHeight);
    }

    const searchVal = () => {
        //if (!finder.length) return
        $('#container span').each(function () {
            if ($(this).text().includes(finder)) {
                let newText = $(this).text().replace(finder, `<i>${finder}</i>`)
                $(this).html(newText)
            }
        });
    }

    const changeTypeLevel = (level) => {
        if (typeLevel !== level) {
            Cookies.set('level', level)
            typeLevel = level
            request_id = 0
            fetchData()
            $('#container').html("")
            data = []
        }
    }

    const initPanel = () => {
        switch (typeLevel) {
            case "debug":
                $("#levelDebug").prop('checked', true);
                break
            case "info":
                $("#levelInfo").prop('checked', true);
                break
            case "warn":
                $("#levelWarn").prop('checked', true);
                break
            case "error":
                $("#levelError").prop('checked', true);
                break
        }
        switch (lineBreak) {
            case "false":
                $("#lineBreak").prop('checked', false);
                changeLineBreak(false)
                break
            default:
                $("#lineBreak").prop('checked', true);
                changeLineBreak(true)
                break
        }
    }

    const changeFollow = (value) => {
        Follow = value
        $("#followBtn").prop('checked', value);
    }

    const changeLineBreak = (value) => {
        if (value) {
            Cookies.set('lineBreak', "true");
            $("#container").removeClass("withoutBreak")
            $("#lineBreakOffIcon").hide()
            $("#lineBreakOnIcon").show()
        } else {
            Cookies.set('lineBreak', "false");
            $("#container").addClass("withoutBreak")
            $("#lineBreakOffIcon").show()
            $("#lineBreakOnIcon").hide()
        }
    }

    const clickedOnScrollbar = (mouseX) => {

      if ($(window).outerWidth() - 15 <= mouseX){
          changeFollow(false)
      }
    }

    //for debug
    const getID = (id) => {
        if (id > 9999) return id
        if (id > 999) return `0${id}`
        if (id > 99) return `00${id}`
        if (id > 9) return `000${id}`
        return `0000${id}`
    }



    //listeners
    $(window).scroll(function (e) {
        if ($(window).scrollTop() >= $(document).height() - $(window).height() - 30) {
            if (!Follow) changeFollow(true)
        }
    })

    $("html").mousedown(function (e) {
        //console.log("mousedown")
        clickedOnScrollbar(e.clientX)
    })

    $(window).on('wheel', function(e){
        //if ($(window).outerHeight() < $(document).outerHeight()){
            changeFollow(false)
        //}
    })

    $("#searchInput").keyup(function () {
        finder = $(this).val();
        searchVal()
    })

    $("#lineBreak").click(function () {
        changeLineBreak(this.checked)
    })

    $("#followBtn").click(function (e) {
        if (this.checked) {
            scrollToBottom();
            Follow = true
        } else {
            Follow = false
        }
    })

    $("#clearBtn").click(function (e) {
        e.preventDefault();
        data = []
        $("#container").html("")
    })

    $("#levelInfo").click(function () {
        changeTypeLevel("info")
    })
    $("#levelWarn").click(function () {
        changeTypeLevel("warn")
    })
    $("#levelDebug").click(function () {
        changeTypeLevel("debug")
    })
    $("#levelError").click(function () {
        changeTypeLevel("error")
    })


    //app
    scrollToBottom()
    fetchData()
    initPanel()

});