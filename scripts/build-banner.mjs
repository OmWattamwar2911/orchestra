import fs from 'fs';
import path from 'path';
import { chromium } from '../web/node_modules/playwright/index.mjs';

const rootDir = process.cwd();
const assetsDir = path.join(rootDir, 'assets');
const fontsDir = path.join(rootDir, 'web', 'public', 'fonts');

// Read font files and base64 encode
const boldFontB64 = fs.readFileSync(path.join(fontsDir, 'GeneralSans-Bold.woff2')).toString('base64');
const regFontB64 = fs.readFileSync(path.join(fontsDir, 'GeneralSans-Regular.woff2')).toString('base64');
const varFontB64 = fs.readFileSync(path.join(fontsDir, 'GeneralSans-Variable.woff2')).toString('base64');

const fontFaceCss = `
@font-face {
  font-family: 'General Sans';
  src: url('data:font/woff2;base64,${boldFontB64}') format('woff2');
  font-weight: 700 900;
  font-style: normal;
}
@font-face {
  font-family: 'General Sans';
  src: url('data:font/woff2;base64,${regFontB64}') format('woff2');
  font-weight: 400;
  font-style: normal;
}
@font-face {
  font-family: 'General Sans';
  src: url('data:font/woff2;base64,${varFontB64}') format('woff2');
  font-weight: 100 900;
  font-style: normal;
}
@font-face {
  font-family: 'JetBrains Mono';
  src: local('JetBrains Mono'), local('Consolas'), monospace;
  font-weight: 400 700;
}
`;

function createBannerHtml({ isDarkWrapper = false, isSocial = false } = {}) {
  const width = isSocial ? 1280 : 1600;
  const height = isSocial ? 640 : 400;
  const topBarHeight = isSocial ? 70 : 80;
  const headlineSize = isSocial ? '52px' : '56px';
  const taglineSize = isSocial ? '15px' : '14px';
  const sidebarWidth = isSocial ? '220px' : '260px';

  const innerHtml = `
  <div class="banner-frame" style="
    width: ${width}px;
    height: ${height}px;
    background-color: #E3E2DE;
    border: 1px solid #C7C7C7;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    position: relative;
    overflow: hidden;
  ">
    <!-- Top Bar -->
    <header style="
      height: ${topBarHeight}px;
      border-bottom: 1px solid #C7C7C7;
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 0 32px;
      box-sizing: border-box;
    ">
      <div style="display: flex; align-items: center; gap: 14px;">
        <div style="width: 16px; height: 16px; background: #141414;"></div>
        <span style="
          font-family: 'General Sans', sans-serif;
          font-weight: 900;
          font-size: 24px;
          letter-spacing: -0.04em;
          color: #141414;
        ">ORCHESTRA</span>
      </div>

      <div style="
        font-family: 'JetBrains Mono', Consolas, monospace;
        font-size: 12px;
        font-weight: 500;
        letter-spacing: 0.12em;
        color: #7A7A7A;
        text-transform: uppercase;
      ">
        SCHEDULER V1.0
      </div>

      <div style="
        display: flex;
        align-items: center;
        gap: 32px;
        font-family: 'General Sans', sans-serif;
        font-weight: 700;
        font-size: 12px;
        letter-spacing: 0.1em;
        color: #141414;
        text-transform: uppercase;
      ">
        <span>DOCS</span>
        <span>DASHBOARD</span>
        <span>GITHUB</span>
      </div>
    </header>

    <!-- Main Grid: Sidebar + Content -->
    <div style="
      display: grid;
      grid-template-columns: ${sidebarWidth} 1fr;
      flex: 1;
      height: ${height - topBarHeight}px;
      box-sizing: border-box;
    ">
      <!-- Left Sidebar (Manifesto) -->
      <aside style="
        border-right: 1px solid #C7C7C7;
        padding: 24px 28px;
        display: flex;
        flex-direction: column;
        justify-content: space-between;
        box-sizing: border-box;
        background-color: #E3E2DE;
      ">
        <div>
          <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px;">
            <span style="
              font-family: 'General Sans', sans-serif;
              font-weight: 900;
              font-size: 12px;
              letter-spacing: 0.16em;
              color: #141414;
              text-transform: uppercase;
            ">MANIFESTO</span>
            <div style="width: 16px; height: 16px; background: #141414;"></div>
          </div>
          <p style="
            font-family: 'JetBrains Mono', Consolas, monospace;
            font-size: 11px;
            line-height: 1.6;
            color: #444343;
            margin: 0;
          ">
            Workflows must survive split-brains, network partitions, and host reboots without human intervention.
          </p>
        </div>

        <div style="
          border-top: 1px solid #C7C7C7;
          padding-top: 16px;
          font-family: 'JetBrains Mono', Consolas, monospace;
          font-size: 10px;
          line-height: 1.6;
          color: #7A7A7A;
          letter-spacing: 0.05em;
        ">
          <div>RAFT CONSENSUS</div>
          <div>FENCING TOKEN CAS</div>
          <div>PROVEN RESILIENCE</div>
        </div>
      </aside>

      <!-- Content Area -->
      <main style="
        padding: 24px 36px 20px 36px;
        display: flex;
        flex-direction: column;
        justify-content: space-between;
        box-sizing: border-box;
        position: relative;
        background-color: #E3E2DE;
      ">
        <!-- Headline & Tagline -->
        <div>
          <h1 style="
            font-family: 'General Sans', sans-serif;
            font-weight: 900;
            font-size: ${headlineSize};
            line-height: 0.88;
            letter-spacing: -0.04em;
            color: #141414;
            margin: 0 0 12px 0;
            padding: 0;
          ">
            Run task graphs.<br>
            <span style="color: #1351AA;">Survive</span> crashes.
          </h1>
          <p style="
            font-family: 'JetBrains Mono', Consolas, monospace;
            font-size: ${taglineSize};
            font-weight: 500;
            color: #444343;
            margin: 0;
            letter-spacing: -0.01em;
          ">
            A fault-tolerant distributed DAG workflow scheduler. Go, etcd, gRPC.
          </p>
        </div>

        <!-- Flat Score Strip -->
        <div style="
          margin-top: ${isSocial ? '24px' : '16px'};
          border: 1px solid #C7C7C7;
          background: #DCD9D4;
          box-sizing: border-box;
          position: relative;
        ">
          <!-- Lane Header -->
          <div style="
            height: 20px;
            background: #D4D0C9;
            border-bottom: 1px solid #C7C7C7;
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 0 12px;
            font-family: 'JetBrains Mono', Consolas, monospace;
            font-size: 9px;
            font-weight: 700;
            letter-spacing: 0.1em;
            color: #555454;
            text-transform: uppercase;
          ">
            <span>LIVE EXECUTION SCORE (5 LANES)</span>
            <span>DAG: FRAUD-DETECTION-092 • LEADER TERM 04</span>
          </div>

          <!-- 5 Horizontal Worker Lanes -->
          <div style="display: flex; flex-direction: column; position: relative;">
            
            <!-- Lane 1 (Worker 01) -->
            <div style="
              height: 24px;
              border-bottom: 1px solid #C7C7C7;
              display: flex;
              align-items: center;
              position: relative;
              background: #E3E2DE;
            ">
              <span style="width: 55px; padding-left: 10px; font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700; color: #7A7A7A;">W-01</span>
              <!-- Task Block: Completed (Solid Jet) -->
              <div style="
                position: absolute; left: 70px; width: 140px; height: 18px;
                background: #141414; color: #E3E2DE;
                display: flex; align-items: center; justify-content: center;
                font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700;
              ">
                INGEST-LOGS
              </div>
              <!-- Task Block: Completed (Solid Jet) -->
              <div style="
                position: absolute; left: 220px; width: 150px; height: 18px;
                background: #141414; color: #E3E2DE;
                display: flex; align-items: center; justify-content: center;
                font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700;
              ">
                SCHEMA-VALIDATE
              </div>
              <!-- Task Block: Queued (Hairline outline) -->
              <div style="
                position: absolute; left: 750px; width: 160px; height: 18px;
                border: 1px solid #7A7A7A; background: transparent; color: #555;
                display: flex; align-items: center; justify-content: center;
                font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 500;
              ">
                ARCHIVE-S3
              </div>
            </div>

            <!-- Lane 2 (Worker 02) -->
            <div style="
              height: 24px;
              border-bottom: 1px solid #C7C7C7;
              display: flex;
              align-items: center;
              position: relative;
              background: #E3E2DE;
            ">
              <span style="width: 55px; padding-left: 10px; font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700; color: #7A7A7A;">W-02</span>
              <!-- Task Block: Completed (Solid Jet) -->
              <div style="
                position: absolute; left: 70px; width: 160px; height: 18px;
                background: #141414; color: #E3E2DE;
                display: flex; align-items: center; justify-content: center;
                font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700;
              ">
                EXTRACT-FEATURES
              </div>

              <!-- Reassigned Task from Worker 04 -->
              <div style="
                position: absolute; left: 560px; width: 170px; height: 18px;
                background: #1351AA; color: #E3E2DE;
                display: flex; align-items: center; justify-content: center;
                font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700;
                box-shadow: 0 0 0 1px #1351AA;
              ">
                SCORE-INFERENCE (REC)
              </div>
            </div>

            <!-- Lane 3 (Worker 03) -->
            <div style="
              height: 24px;
              border-bottom: 1px solid #C7C7C7;
              display: flex;
              align-items: center;
              position: relative;
              background: #E3E2DE;
            ">
              <span style="width: 55px; padding-left: 10px; font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700; color: #7A7A7A;">W-03</span>
              <!-- Task Block: Retry hatched with Amber border -->
              <div style="
                position: absolute; left: 240px; width: 150px; height: 18px;
                background: repeating-linear-gradient(45deg, #E3E2DE, #E3E2DE 4px, #FFF3DC 4px, #FFF3DC 8px);
                border: 1px solid #B86E00; color: #B86E00;
                display: flex; align-items: center; justify-content: center;
                font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700;
              ">
                TOKENIZE • RETRY #1
              </div>
              <!-- Running Task: Solid Cobalt -->
              <div style="
                position: absolute; left: 400px; width: 150px; height: 18px;
                background: #1351AA; color: #E3E2DE;
                display: flex; align-items: center; justify-content: center;
                font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700;
              ">
                TOKENIZE • ATTEMPT 2
              </div>
            </div>

            <!-- Lane 4 (Worker 04 - Dead / Stopped Worker) -->
            <div style="
              height: 24px;
              border-bottom: 1px solid #C7C7C7;
              display: flex;
              align-items: center;
              position: relative;
              background: #ECEAE6;
            ">
              <span style="width: 55px; padding-left: 10px; font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700; color: #B82020;">W-04 ✕</span>
              
              <!-- Stopped Task Block -->
              <div style="
                position: absolute; left: 380px; width: 150px; height: 18px;
                background: #FADBD8; border: 1px dashed #B82020; color: #900C3F;
                display: flex; align-items: center; justify-content: center;
                font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700;
              ">
                SCORE-INFERENCE ✕
              </div>
            </div>

            <!-- Lane 5 (Worker 05) -->
            <div style="
              height: 24px;
              display: flex;
              align-items: center;
              position: relative;
              background: #E3E2DE;
            ">
              <span style="width: 55px; padding-left: 10px; font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700; color: #7A7A7A;">W-05</span>
              <!-- Task Block: Completed (Solid Jet) -->
              <div style="
                position: absolute; left: 70px; width: 180px; height: 18px;
                background: #141414; color: #E3E2DE;
                display: flex; align-items: center; justify-content: center;
                font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700;
              ">
                BUILD-USER-GRAPH
              </div>
              <!-- Running Task: Solid Cobalt -->
              <div style="
                position: absolute; left: 260px; width: 180px; height: 18px;
                background: #1351AA; color: #E3E2DE;
                display: flex; align-items: center; justify-content: center;
                font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 700;
              ">
                CLUSTERING-EMBED
              </div>
              <!-- Queued Task -->
              <div style="
                position: absolute; left: 450px; width: 130px; height: 18px;
                border: 1px solid #7A7A7A; background: transparent; color: #555;
                display: flex; align-items: center; justify-content: center;
                font-family: 'JetBrains Mono', monospace; font-size: 9px; font-weight: 500;
              ">
                NOTIFY-SLACK
              </div>
            </div>

            <!-- SVG Connector between Lane 4 and Lane 2 -->
            <svg style="position: absolute; top: 0; left: 0; width: 100%; height: 120px; pointer-events: none;">
              <!-- Path from (530px, 84px) [Lane 4 center] curving up to (560px, 36px) [Lane 2 center] -->
              <path d="M 530 84 C 545 84, 545 36, 560 36" fill="none" stroke="#1351AA" stroke-width="2" stroke-dasharray="3,3" />
              <!-- Small arrow or marker -->
              <circle cx="530" cy="84" r="3" fill="#B82020" />
              <polygon points="560,33 566,36 560,39" fill="#1351AA" />
              <!-- Callout Pill -->
              <rect x="475" y="50" width="135" height="15" fill="#1351AA" />
              <text x="542" y="61" fill="#E3E2DE" font-family="'JetBrains Mono', monospace" font-size="8" font-weight="700" text-anchor="middle">
                worker stopped, task moved
              </text>
            </svg>

          </div>
        </div>

      </main>
    </div>
  </div>
  `;

  if (isDarkWrapper) {
    return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Orchestra Banner Dark Wrapper</title>
  <style>
    ${fontFaceCss}
    * { box-sizing: border-box; }
    body {
      margin: 0;
      padding: 40px;
      background-color: #0D1117;
      display: flex;
      align-items: center;
      justify-content: center;
      width: ${width + 80}px;
      height: ${height + 80}px;
    }
  </style>
</head>
<body>
  ${innerHtml}
</body>
</html>`;
  }

  return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Orchestra Banner</title>
  <style>
    ${fontFaceCss}
    * { box-sizing: border-box; }
    body {
      margin: 0;
      padding: 0;
      background-color: transparent;
      width: ${width}px;
      height: ${height}px;
      display: flex;
    }
  </style>
</head>
<body>
  ${innerHtml}
</body>
</html>`;
}

// Generate HTML files
const bannerHtml = createBannerHtml();
const bannerDarkHtml = createBannerHtml({ isDarkWrapper: true });
const socialPreviewHtml = createBannerHtml({ isSocial: true });

fs.writeFileSync(path.join(assetsDir, 'banner.html'), bannerHtml);
fs.writeFileSync(path.join(assetsDir, 'banner-dark.html'), bannerDarkHtml);
fs.writeFileSync(path.join(assetsDir, 'social-preview.html'), socialPreviewHtml);

console.log('Generated banner HTML source files in assets/');

async function renderBanners() {
  const browser = await chromium.launch({ channel: 'msedge' });
  
  // 1. Render Banner Light (1600x400 @ 2x = 3200x800)
  const pageLight = await browser.newPage({
    viewport: { width: 1600, height: 400 },
    deviceScaleFactor: 2
  });
  await pageLight.goto(`file://${path.join(assetsDir, 'banner.html')}`);
  await pageLight.evaluate(async () => {
    await document.fonts.ready;
    const fontLoaded = document.fonts.check('900 16px "General Sans"');
    if (!fontLoaded) {
      throw new Error("CRITICAL: document.fonts.check('900 16px \"General Sans\"') returned false!");
    }
  });
  await pageLight.screenshot({
    path: path.join(assetsDir, 'banner-light.png'),
    clip: { x: 0, y: 0, width: 1600, height: 400 }
  });
  console.log('✓ Exported assets/banner-light.png (3200x800)');
  await pageLight.close();

  // 2. Render Banner Dark (1680x480 @ 2x = 3360x960 with #0D1117 padding)
  const pageDark = await browser.newPage({
    viewport: { width: 1680, height: 480 },
    deviceScaleFactor: 2
  });
  await pageDark.goto(`file://${path.join(assetsDir, 'banner-dark.html')}`);
  await pageDark.evaluate(async () => {
    await document.fonts.ready;
    const fontLoaded = document.fonts.check('900 16px "General Sans"');
    if (!fontLoaded) {
      throw new Error("CRITICAL: document.fonts.check('900 16px \"General Sans\"') returned false!");
    }
  });
  await pageDark.screenshot({
    path: path.join(assetsDir, 'banner-dark.png'),
    clip: { x: 0, y: 0, width: 1680, height: 480 }
  });
  console.log('✓ Exported assets/banner-dark.png (3360x960 with dark frame)');
  await pageDark.close();

  // 3. Render Social Preview (1280x640 @ 2x = 2560x1280)
  const pageSocial = await browser.newPage({
    viewport: { width: 1280, height: 640 },
    deviceScaleFactor: 2
  });
  await pageSocial.goto(`file://${path.join(assetsDir, 'social-preview.html')}`);
  await pageSocial.evaluate(async () => {
    await document.fonts.ready;
    const fontLoaded = document.fonts.check('900 16px "General Sans"');
    if (!fontLoaded) {
      throw new Error("CRITICAL: document.fonts.check('900 16px \"General Sans\"') returned false!");
    }
  });
  await pageSocial.screenshot({
    path: path.join(assetsDir, 'social-preview.png'),
    clip: { x: 0, y: 0, width: 1280, height: 640 }
  });
  console.log('✓ Exported assets/social-preview.png (2560x1280)');
  await pageSocial.close();

  await browser.close();
  console.log('All banner exports completed successfully.');
}

renderBanners().catch(err => {
  console.error('Error rendering banners:', err);
  process.exit(1);
});
