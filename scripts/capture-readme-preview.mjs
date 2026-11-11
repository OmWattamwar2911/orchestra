import fs from 'fs';
import path from 'path';
import { chromium } from '../web/node_modules/playwright/index.mjs';

const rootDir = process.cwd();

function createPreviewHtml(theme = 'light') {
  const isDark = theme === 'dark';
  const bgColor = isDark ? '#0d1117' : '#ffffff';
  const textColor = isDark ? '#e6edf3' : '#1f2328';
  const borderColor = isDark ? '#30363d' : '#d0d7de';
  const quoteColor = isDark ? '#8d96a0' : '#656d76';

  const bannerImg = isDark ? 'assets/banner-dark.png' : 'assets/banner-light.png';

  return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>README Preview - ${theme}</title>
  <style>
    body {
      background-color: ${bgColor};
      color: ${textColor};
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans", Helvetica, Arial, sans-serif;
      margin: 0;
      padding: 32px;
      display: flex;
      justify-content: center;
    }
    .markdown-body {
      max-width: 980px;
      width: 100%;
      box-sizing: border-box;
    }
    blockquote {
      margin: 16px 0;
      padding: 0 1em;
      color: ${quoteColor};
      border-left: 0.25em solid ${borderColor};
      font-size: 16px;
    }
    img {
      max-width: 100%;
      box-sizing: content-box;
    }
    p {
      margin-top: 0;
      margin-bottom: 16px;
    }
    hr {
      height: 0.25em;
      padding: 0;
      margin: 24px 0;
      background-color: ${borderColor};
      border: 0;
    }
  </style>
</head>
<body>
  <article class="markdown-body">
    <p align="center">
      <img src="${bannerImg}" alt="Orchestra Banner" width="100%">
    </p>

    <p align="center">
      <img src="https://img.shields.io/badge/Go-1.22-1351AA?style=flat-square&logo=go&logoColor=E3E2DE" alt="Go Version">
      <img src="https://img.shields.io/badge/License-MIT-141414?style=flat-square&color=141414&labelColor=E3E2DE" alt="License: MIT">
      <img src="https://img.shields.io/badge/CI-Passing-1351AA?style=flat-square&logo=githubactions&logoColor=E3E2DE" alt="CI Status">
      <img src="https://img.shields.io/badge/Design-Poster_Modernist-141414?style=flat-square&color=141414&labelColor=1351AA" alt="Design: Poster Modernist">
    </p>

    <blockquote>
      <strong>A distributed workflow scheduler that proves, with an independent chaos checker, it survives crashes without violating task order or losing tasks.</strong>
    </blockquote>

    <p align="center">
      <img src="docs/demo.gif" alt="Orchestra Demo Walkthrough" width="100%">
    </p>

    <hr>
  </article>
</body>
</html>`;
}

async function render() {
  const browser = await chromium.launch({ channel: 'msedge' });
  
  // Light Preview
  fs.writeFileSync(path.join(rootDir, 'preview-light.html'), createPreviewHtml('light'));
  const pageLight = await browser.newPage({ viewport: { width: 1200, height: 950 }, deviceScaleFactor: 2 });
  await pageLight.goto(`file://${path.join(rootDir, 'preview-light.html')}`, { waitUntil: 'networkidle' });
  await pageLight.waitForTimeout(1000);
  await pageLight.screenshot({ path: path.join(rootDir, 'assets', 'readme-preview-light.png') });
  await pageLight.close();
  fs.unlinkSync(path.join(rootDir, 'preview-light.html'));
  console.log('✓ Exported assets/readme-preview-light.png');

  // Dark Preview
  fs.writeFileSync(path.join(rootDir, 'preview-dark.html'), createPreviewHtml('dark'));
  const pageDark = await browser.newPage({ viewport: { width: 1200, height: 950 }, deviceScaleFactor: 2 });
  await pageDark.goto(`file://${path.join(rootDir, 'preview-dark.html')}`, { waitUntil: 'networkidle' });
  await pageDark.waitForTimeout(1000);
  await pageDark.screenshot({ path: path.join(rootDir, 'assets', 'readme-preview-dark.png') });
  await pageDark.close();
  fs.unlinkSync(path.join(rootDir, 'preview-dark.html'));
  console.log('✓ Exported assets/readme-preview-dark.png');

  await browser.close();
}

render().catch(console.error);
