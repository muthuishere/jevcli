#!/usr/bin/env node
// Runs the jevx binary downloaded by install.js, passing arguments, stdin/stdout and the exit code straight through.
'use strict';
const path = require('path');
const fs = require('fs');
const { spawnSync } = require('child_process');
const bin = path.join(__dirname, 'jevx-bin' + (process.platform === 'win32' ? '.exe' : ''));
if (!fs.existsSync(bin)) {
  // npm skips install scripts when they are not approved (allow-scripts): fetch the binary now, once.
  spawnSync(process.execPath, [path.join(__dirname, '..', 'install.js')], { stdio: 'inherit' });
  if (!fs.existsSync(bin)) {
    console.error('jevx: binary missing; install with: curl -fsSL https://muthuishere.github.io/jevx/install.sh | sh');
    process.exit(4);
  }
}
const r = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' });
process.exit(r.status === null ? 4 : r.status);
