import http from 'node:http'
import net from 'node:net'

const smtpPort = Number(process.env.OPEN_CRM_E2E_SMTP_PORT || 2525)
const httpPort = Number(process.env.OPEN_CRM_E2E_SMTP_HTTP_PORT || 2526)
const host = '127.0.0.1'
const maxMessageBytes = 1024 * 1024
const maxMessages = 100

let messages = []

function writeLine(socket, line) {
  socket.write(`${line}\r\n`)
}

const smtpServer = net.createServer((socket) => {
  socket.setEncoding('utf8')
  socket.setTimeout(30_000, () => socket.destroy())

  let input = ''
  let collectingData = false
  let dataLines = []
  let dataBytes = 0
  let envelopeFrom = ''
  let envelopeTo = ''

  function resetEnvelope() {
    collectingData = false
    dataLines = []
    dataBytes = 0
    envelopeFrom = ''
    envelopeTo = ''
  }

  function finishData() {
    const data = `${dataLines.join('\r\n')}\r\n`
    if (dataBytes > maxMessageBytes) {
      writeLine(socket, '552 5.3.4 Message exceeds test sandbox limit')
      resetEnvelope()
      return
    }
    messages.push({
      envelopeFrom,
      envelopeTo,
      data,
      acceptedAt: new Date().toISOString()
    })
    if (messages.length > maxMessages) messages = messages.slice(-maxMessages)
    writeLine(socket, '250 2.0.0 Accepted by Open CRM test sandbox')
    resetEnvelope()
  }

  function handleCommand(line) {
    if (collectingData) {
      if (line === '.') {
        finishData()
        return
      }
      const unescaped = line.startsWith('..') ? line.slice(1) : line
      dataBytes += Buffer.byteLength(unescaped) + 2
      if (dataBytes <= maxMessageBytes) dataLines.push(unescaped)
      return
    }

    const space = line.indexOf(' ')
    const command = (space === -1 ? line : line.slice(0, space)).toUpperCase()
    const argument = space === -1 ? '' : line.slice(space + 1)
    switch (command) {
      case 'EHLO':
        writeLine(socket, '250-localhost')
        writeLine(socket, `250 SIZE ${maxMessageBytes}`)
        break
      case 'HELO':
        writeLine(socket, '250 localhost')
        break
      case 'MAIL':
        envelopeFrom = argument.replace(/^FROM:\s*/i, '')
        writeLine(socket, '250 2.1.0 Sender accepted')
        break
      case 'RCPT':
        envelopeTo = argument.replace(/^TO:\s*/i, '')
        writeLine(socket, '250 2.1.5 Recipient accepted')
        break
      case 'DATA':
        if (!envelopeFrom || !envelopeTo) {
          writeLine(socket, '503 5.5.1 MAIL and RCPT required')
          break
        }
        collectingData = true
        dataLines = []
        dataBytes = 0
        writeLine(socket, '354 End data with <CR><LF>.<CR><LF>')
        break
      case 'RSET':
        resetEnvelope()
        writeLine(socket, '250 2.0.0 Reset')
        break
      case 'NOOP':
        writeLine(socket, '250 2.0.0 OK')
        break
      case 'QUIT':
        socket.end('221 2.0.0 Bye\r\n')
        break
      default:
        writeLine(socket, '502 5.5.2 Command not implemented')
    }
  }

  writeLine(socket, '220 localhost Open CRM test SMTP ready')
  socket.on('data', (chunk) => {
    input += chunk
    let newline = input.indexOf('\n')
    while (newline !== -1) {
      const line = input.slice(0, newline).replace(/\r$/, '')
      input = input.slice(newline + 1)
      handleCommand(line)
      newline = input.indexOf('\n')
    }
  })
})

const httpServer = http.createServer((request, response) => {
  response.setHeader('Cache-Control', 'no-store')
  response.setHeader('Content-Type', 'application/json; charset=utf-8')
  if (request.method === 'GET' && request.url === '/health') {
    response.end(JSON.stringify({ status: 'ok' }))
    return
  }
  if (request.method === 'GET' && request.url === '/messages') {
    response.end(JSON.stringify({ messages }))
    return
  }
  if (request.method === 'DELETE' && request.url === '/messages') {
    messages = []
    response.end(JSON.stringify({ messages: [] }))
    return
  }
  response.statusCode = 404
  response.end(JSON.stringify({ error: 'not found' }))
})

smtpServer.listen(smtpPort, host)
httpServer.listen(httpPort, host)

function shutdown() {
  smtpServer.close()
  httpServer.close(() => process.exit(0))
}

process.on('SIGINT', shutdown)
process.on('SIGTERM', shutdown)
