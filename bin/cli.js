#!/usr/bin/env node

const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');
const os = require('os');

// Fixed nacos-cli binary version
// npm package version is independent from binary version
const VERSION = '0.0.5';

// Detect platform and architecture
function getBinaryName() {
  const platform = os.platform();
  const arch = os.arch();

  let platformName;
  let archName;
  let ext = '';

  // Map Node.js platform to Go platform names
  switch (platform) {
    case 'darwin':
      platformName = 'darwin';
      break;
    case 'linux':
      platformName = 'linux';
      break;
    case 'win32':
      platformName = 'windows';
      ext = '.exe';
      break;
    default:
      console.error(`Unsupported platform: ${platform}`);
      process.exit(1);
  }

  // Map Node.js arch to Go arch names
  switch (arch) {
    case 'x64':
      archName = 'amd64';
      break;
    case 'arm64':
      archName = 'arm64';
      break;
    default:
      console.error(`Unsupported architecture: ${arch}`);
      process.exit(1);
  }

  return `nacos-cli-${VERSION}-${platformName}-${archName}${ext}`;
}

// Get binary path
function getBinaryPath() {
  const binaryName = getBinaryName();
  const binaryPath = path.join(__dirname, '..', 'build', binaryName);

  if (!fs.existsSync(binaryPath)) {
    console.error(`Binary not found: ${binaryPath}`);
    console.error(`Please ensure the binary for your platform (${os.platform()}/${os.arch()}) is available.`);
    process.exit(1);
  }

  return binaryPath;
}

// Main execution
function main() {
  const binaryPath = getBinaryPath();
  const args = process.argv.slice(2);

  // Spawn the binary with all arguments
  const child = spawn(binaryPath, args, {
    stdio: 'inherit',
    shell: false
  });

  child.on('error', (err) => {
    console.error(`Failed to execute binary: ${err.message}`);
    process.exit(1);
  });

  child.on('exit', (code) => {
    process.exit(code || 0);
  });
}

main();
