// Put the following global function to your project.
function repl(__tunnelURL) {
  function inspect(object) {
    return Object.keys(object).sort().map(
      (function(k){
        return typeof object[k] + " [" + k + "] = " + object[k];
      }));
  }
  
  function log(msg) {
    UrlFetchApp.fetch(
      __tunnelURL,
      {
        'method': 'post',
        'contentType': 'text/plain',
        'payload': '__LOG ' + msg
      }
    );
  }
  
  let __value = '__READY';
  do {
    Logger.log(`${__value}`);
    
    const __response = UrlFetchApp.fetch(
      __tunnelURL,
      {
        'method': 'post',
        'contentType': 'text/plain',
        'payload': __value
      }
    ).getContentText();
    
    Logger.log(`> ${__response}`);
    
    if (__response == "__USER_INPUT_TIMEOUT") {
      __value = '__KEEPALIVE';
      continue;
    }
    
    if (__response == "__DISCONNECT") {
      break;
    }
    
    if (/^__DESCRIBE .+$/.test(__response)) {
      try {
        let __varName = __response.match(/^__DESCRIBE (.+)$/)[1];
        const __propNames = Object.getOwnPropertyNames(eval(__varName));
        const __properties = __propNames.map(__propName => (
          {
            name: __propName,
            type: typeof eval(__varName)[__propName]
          }
        ));
        __value = JSON.stringify(__properties);
      } catch(__error) {
        Logger.log(__error);
        __value = '[]';
      }
      continue;
    }
    
    try {
      __value = JSON.stringify(eval(__response), null, 2);
    } catch (__error) {
      Logger.log(__error);
      __value = __error
    }
    
  } while (true);
}


function start() {
  repl('http://example.com');
}
