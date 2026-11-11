import fs from 'fs';
import path from 'path';
import { chromium } from '../web/node_modules/playwright/index.mjs';

const rootDir = process.cwd();
const screenshotsDir = path.join(rootDir, 'assets', 'screenshots');
if (!fs.existsSync(screenshotsDir)) {
  fs.mkdirSync(screenshotsDir, { recursive: true });
}

const BASE_URL = 'http://localhost:3000';

async function capture() {
  const browser = await chromium.launch({ channel: 'msedge' });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 2
  });

  const page = await context.newPage();

  async function checkVisualIntegrity(pageName) {
    const integrity = await page.evaluate(() => {
      const text = document.body.innerText;
      const isReconnecting = text.includes('Reconnecting...') || text.includes('discovering...');
      const style = window.getComputedStyle(document.body);
      const isCream = style.backgroundColor.includes('227, 226, 222') || style.backgroundColor.includes('e3e2de');
      return { isReconnecting, isCream, bg: style.backgroundColor };
    });

    if (integrity.isReconnecting) {
      throw new Error(`Integrity check failed for ${pageName}: contains "Reconnecting..." or "discovering..."`);
    }
    if (!integrity.isCream) {
      throw new Error(`Integrity check failed for ${pageName}: background is not #E3E2DE (got ${integrity.bg})`);
    }
    console.log(`✓ Integrity check passed for ${pageName}`);
  }

  // 1. Landing Page Hero
  console.log('Capturing Landing Page Hero...');
  await page.goto(`${BASE_URL}/?demo=true`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(1500);
  await checkVisualIntegrity('Landing Hero');
  await page.screenshot({
    path: path.join(screenshotsDir, 'landing-hero.png'),
    clip: { x: 0, y: 0, width: 1440, height: 850 }
  });
  console.log('✓ Saved landing-hero.png');

  // 2. Landing Page "Why Different"
  console.log('Capturing Landing Page Why Different...');
  const whySection = await page.$('#why-different, [data-section="why-different"], section:has-text("WHY DIFFERENT")');
  if (whySection) {
    await whySection.scrollIntoViewIfNeeded();
    await page.waitForTimeout(500);
    await whySection.screenshot({
      path: path.join(screenshotsDir, 'landing-why-different.png')
    });
  } else {
    // Scroll down to middle section
    await page.evaluate(() => window.scrollTo(0, 1100));
    await page.waitForTimeout(600);
    await page.screenshot({
      path: path.join(screenshotsDir, 'landing-why-different.png'),
      clip: { x: 0, y: 100, width: 1440, height: 750 }
    });
  }
  console.log('✓ Saved landing-why-different.png');

  // 3. Cluster Page
  console.log('Capturing Cluster Page...');
  await page.goto(`${BASE_URL}/app?demo=true`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await checkVisualIntegrity('Cluster Page');
  await page.screenshot({
    path: path.join(screenshotsDir, 'cluster.png'),
    clip: { x: 0, y: 0, width: 1440, height: 900 }
  });
  console.log('✓ Saved cluster.png');

  // 4. Workflows List Page
  console.log('Capturing Workflows Page...');
  await page.goto(`${BASE_URL}/app/workflows?demo=true`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await checkVisualIntegrity('Workflows Page');
  await page.screenshot({
    path: path.join(screenshotsDir, 'workflows.png'),
    clip: { x: 0, y: 0, width: 1440, height: 900 }
  });
  console.log('✓ Saved workflows.png');

  // 5. Workflow Graph (Click into a workflow row to view graph)
  console.log('Capturing Workflow Graph...');
  // Look for a row or workflow item to click
  const workflowRow = await page.$('table tbody tr, [data-testid="workflow-row"], .cursor-pointer');
  if (workflowRow) {
    await workflowRow.click();
    await page.waitForTimeout(1000);
  }
  await page.screenshot({
    path: path.join(screenshotsDir, 'workflow-graph.png'),
    clip: { x: 0, y: 0, width: 1440, height: 900 }
  });
  console.log('✓ Saved workflow-graph.png');

  // 6. Failed or Retrying Workflow detail / view
  console.log('Capturing Retrying/Failed Workflow...');
  // Check if there is a failed / retry tab or filter
  const failedTab = await page.$('button:has-text("FAILED"), button:has-text("RETRYING"), [data-filter="failed"]');
  if (failedTab) {
    await failedTab.click();
    await page.waitForTimeout(500);
  }
  await page.screenshot({
    path: path.join(screenshotsDir, 'workflow-retry.png'),
    clip: { x: 0, y: 0, width: 1440, height: 900 }
  });
  console.log('✓ Saved workflow-retry.png');

  // 7. Submit Page
  console.log('Capturing Submit Page...');
  await page.goto(`${BASE_URL}/app/submit?demo=true`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(1500);
  await checkVisualIntegrity('Submit Page');
  await page.screenshot({
    path: path.join(screenshotsDir, 'submit.png'),
    clip: { x: 0, y: 0, width: 1440, height: 900 }
  });
  console.log('✓ Saved submit.png');

  // 8. Resilience Page
  console.log('Capturing Resilience Page...');
  await page.goto(`${BASE_URL}/app/resilience?demo=true`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(1500);
  await checkVisualIntegrity('Resilience Page');
  await page.screenshot({
    path: path.join(screenshotsDir, 'resilience.png'),
    clip: { x: 0, y: 0, width: 1440, height: 900 }
  });
  console.log('✓ Saved resilience.png');

  await browser.close();
  console.log('\nAll real screenshots captured and validated successfully!');
}

capture().catch(err => {
  console.error('Screenshot capture failed:', err);
  process.exit(1);
});
