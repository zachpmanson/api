package tracker

const homepage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>zachmanson.com API</title>
<style>body{background:#11151d;color:#e6e9ef;font:16px system-ui;max-width:640px;margin:4rem auto;padding:0 1.5rem}a{color:#b5d8ff}li{margin:1.5rem 0}p{line-height:1.5}</style></head>
<body><main><h1>zachmanson.com API</h1>
<p>A small general-purpose API backing a few of my projects. There is nothing at the root itself — the useful bits are the endpoints below.</p>
<h2>Endpoints</h2><ul>
<li><code>GET <a href="/api/prod-broke">/api/prod-broke</a></code><p>Latest broken deployment per environment (raw JSON).</p></li>
<li><code>GET <a href="/api/prod-broke/messages">/api/prod-broke/messages</a></code><p>Human-readable “N days since last broken deployment” per environment.</p></li>
</ul><p><a href="https://github.com/zachpmanson/api">github.com/zachpmanson/api</a></p></main></body></html>`
