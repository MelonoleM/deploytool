const http = require('http');
const args = process.argv.slice(2);
const option = name => args[args.indexOf(name) + 1];
const port = Number(option('--port'));
const name = option('--name');
http.createServer((req, res) => {
  if (req.url === '/' || req.url === '/healthz') {
    res.writeHead(200, {'Content-Type': 'application/json'});
    return res.end(JSON.stringify({service: name, status: 'ok'}));
  }
  res.writeHead(404); res.end();
}).listen(port, '127.0.0.1');
