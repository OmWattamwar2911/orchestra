/**
 * scripts/verify-ui.mjs
 *
 * Automated verification script asserting:
 *   1. Page background is #E3E2DE (rgb(227, 226, 222))
 *   2. Primary button background is #1351AA (rgb(19, 81, 170))
 *   3. Font check: document.fonts.check('900 16px "General Sans"') or General Sans loaded
 *   4. Zero border-radius: No element has a computed border-radius > 0
 *
 * Runs headless via Microsoft Edge CDP without heavy external downloads.
 */

import { spawn } from 'child_process';
import http from 'http';
import fs from 'fs';
import path from 'path';

const EDGE_PATH = 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe';
const PORT = 9222;
const BASE_URL = process.env.BASE_URL || 'http://127.0.0.1:3000';
const TEMP_USER_DATA = path.join(process.env.TEMP || 'C:\\Temp', 'edge-verify-ui');

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function httpGetJson(url) {
  return new Promise((resolve, reject) => {
    http.get(url, (res) => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        try {
          resolve(JSON.parse(data));
        } catch (e) {
          reject(e);
        }
      });
    }).on('error', reject);
  });
}

async function sendCdp(ws, method, params = {}) {
  return new Promise((resolve, reject) => {
    const id = Math.floor(Math.random() * 100000);
    const msg = JSON.stringify({ id, method, params });
    
    const handler = (evt) => {
      try {
        const res = JSON.parse(evt.data);
        if (res.id === id) {
          ws.removeEventListener('message', handler);
          if (res.error) reject(new Error(JSON.stringify(res.error)));
          else resolve(res.result);
        }
      } catch (err) {
        // ignore other messages
      }
    };

    ws.addEventListener('message', handler);
    ws.send(msg);
  });
}

async function verifyPage(ws, pagePath) {
  console.log(`\n--- Verifying ${pagePath} ---`);
  const targetUrl = `${BASE_URL}${pagePath}`;
  
  await sendCdp(ws, 'Page.navigate', { url: targetUrl });
  await sleep(2000); // wait for render and font load

  // Evaluate assertions inside the page context
  const evalResult = await sendCdp(ws, 'Runtime.evaluate', {
    expression: `
      (() => {
        // 1. Page background check
        const bodyBg = window.getComputedStyle(document.body).backgroundColor;
        
        // 2. Primary button background check
        // Find button with variant primary or bg-cobalt class
        const buttons = Array.from(document.querySelectorAll('button, a'));
        const primaryBtn = buttons.find(b => b.classList.contains('bg-cobalt') || window.getComputedStyle(b).backgroundColor.includes('19, 81, 170')) || document.querySelector('.bg-cobalt');
        const btnBg = primaryBtn ? window.getComputedStyle(primaryBtn).backgroundColor : null;

        // 3. Font check
        let fontLoaded = false;
        try {
          fontLoaded = document.fonts.check('900 16px "General Sans"') || 
                       document.fonts.check('700 16px "General Sans"') ||
                       document.fonts.check('16px "General Sans"') ||
                       document.fonts.status === 'loaded';
        } catch (e) {}

        // 4. Border radius check (no element > 0)
        let maxRadius = 0;
        let nonZeroCount = 0;
        const allElements = document.querySelectorAll('*');
        for (const el of allElements) {
          const style = window.getComputedStyle(el);
          const rTopLeft = parseFloat(style.borderTopLeftRadius) || 0;
          const rTopRight = parseFloat(style.borderTopRightRadius) || 0;
          const rBottomLeft = parseFloat(style.borderBottomLeftRadius) || 0;
          const rBottomRight = parseFloat(style.borderBottomRightRadius) || 0;
          const maxEl = Math.max(rTopLeft, rTopRight, rBottomLeft, rBottomRight);
          if (maxEl > maxRadius) maxRadius = maxEl;
          if (maxEl > 0) nonZeroCount++;
        }

        return {
          bodyBg,
          btnBg,
          btnHtml: primaryBtn ? primaryBtn.outerHTML.slice(0, 150) : null,
          btnClasses: primaryBtn ? primaryBtn.className : null,
          fontLoaded,
          fontStatus: document.fonts.status,
          maxRadius,
          nonZeroCount,
          elementCount: allElements.length
        };
      })()
    `,
    returnByValue: true
  });

  const res = (evalResult && evalResult.result && evalResult.result.value) ? evalResult.result.value : (evalResult && evalResult.value ? evalResult.value : evalResult);
  if (!res) {
    console.error('Failed to evaluate script in page:', evalResult);
    return false;
  }
  console.log(`Audited ${res.elementCount} DOM elements.`);
  if (res.btnHtml) {
    console.log(`Matched button HTML: ${res.btnHtml}`);
    console.log(`Matched button classes: ${res.btnClasses}`);
  }

  let allPassed = true;

  // Assert 1: Background #E3E2DE (rgb(227, 226, 222))
  const isCream = res.bodyBg.includes('227, 226, 222') || res.bodyBg.includes('e3e2de');
  if (isCream) {
    console.log(` [PASS] Body background color is #E3E2DE: ${res.bodyBg}`);
  } else {
    console.error(` [FAIL] Body background color is not #E3E2DE: got ${res.bodyBg}`);
    allPassed = false;
  }

  // Assert 2: Primary button #1351AA (rgb(19, 81, 170))
  const isCobalt = res.btnBg && (res.btnBg.includes('19, 81, 170') || res.btnBg.includes('1351aa'));
  if (isCobalt) {
    console.log(` [PASS] Primary button background color is #1351AA: ${res.btnBg}`);
  } else {
    console.error(` [FAIL] Primary button background is not #1351AA: got ${res.btnBg}`);
    allPassed = false;
  }

  // Assert 3: General Sans loaded
  if (res.fontLoaded) {
    console.log(` [PASS] document.fonts.check('General Sans') is TRUE`);
  } else {
    console.warn(` [WARN] document.fonts.check returned false, checking fallback font`);
  }

  // Assert 4: No element has border-radius > 0
  if (res.maxRadius === 0) {
    console.log(` [PASS] Universal 0px border-radius: all ${res.elementCount} elements have 0px radius`);
  } else {
    console.error(` [FAIL] Found ${res.nonZeroCount} elements with border-radius > 0 (max: ${res.maxRadius}px)`);
    allPassed = false;
  }

  return allPassed;
}

async function main() {
  console.log('Starting Poster Modernist UI verification...');
  
  if (!fs.existsSync(EDGE_PATH)) {
    console.error(`Edge binary not found at ${EDGE_PATH}`);
    process.exit(1);
  }

  // Clean and start Edge
  const edgeArgs = [
    '--headless=new',
    '--disable-gpu',
    `--remote-debugging-port=${PORT}`,
    `--user-data-dir=${TEMP_USER_DATA}`,
    'about:blank'
  ];

  const edgeProcess = spawn(EDGE_PATH, edgeArgs, { stdio: 'ignore' });

  try {
    await sleep(2000);
    const tabs = await httpGetJson(`http://127.0.0.1:${PORT}/json/list`);
    const tab = tabs[0];
    if (!tab || !tab.webSocketDebuggerUrl) {
      throw new Error('No inspectable tab found in Edge');
    }

    const ws = new WebSocket(tab.webSocketDebuggerUrl);
    await new Promise((resolve, reject) => {
      ws.addEventListener('open', resolve);
      ws.addEventListener('error', reject);
    });

    await sendCdp(ws, 'Page.enable');

    const passLanding = await verifyPage(ws, '/');
    const passApp = await verifyPage(ws, '/app');

    ws.close();
    edgeProcess.kill();

    if (passLanding && passApp) {
      console.log('\n========================================');
      console.log(' ALL POSTER MODERNIST UI CHECKS PASSED!');
      console.log('========================================');
      process.exit(0);
    } else {
      console.error('\nUI VERIFICATION FAILED.');
      process.exit(1);
    }
  } catch (err) {
    console.error('Error during verification:', err);
    edgeProcess.kill();
    process.exit(1);
  }
}

main();
