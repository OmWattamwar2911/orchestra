/**
 * scripts/record-demo.ts
 *
 * Automated Playwright demo recording script for Orchestra.
 * Executes the signature end-to-end user journey:
 *   1. Landing Page (Manifesto & Score View)
 *   2. Cluster Overview (/app)
 *   3. Workflow Submission (/app/submit - Diamond Pattern)
 *   4. DAG Visual Execution (/app/workflows)
 *   5. Resilience Lab (/app/resilience - Kill Leader Mid-Flight & Recovery)
 *
 * To run:
 *   npx playwright test scripts/record-demo.ts
 *   or: npx ts-node scripts/record-demo.ts
 */

import { chromium, Browser, Page } from 'playwright';
import * as path from 'path';
import * as fs from 'fs';

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const OUTPUT_DIR = path.resolve(__dirname, '../docs/demo-recording');

async function runDemo() {
  if (!fs.existsSync(OUTPUT_DIR)) {
    fs.mkdirSync(OUTPUT_DIR, { recursive: true });
  }

  console.log(`Starting Orchestra automated demo recording on ${BASE_URL}...`);
  
  const browser: Browser = await chromium.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox']
  });

  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    recordVideo: {
      dir: OUTPUT_DIR,
      size: { width: 1440, height: 900 },
    },
  });

  const page: Page = await context.newPage();

  try {
    // SCENE 1: Landing Page
    console.log('[Scene 1/5] Loading Landing Page (/)...');
    await page.goto(`${BASE_URL}/`, { waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);
    await page.screenshot({ path: path.join(OUTPUT_DIR, 'step1-landing.png') });

    // SCENE 2: Cluster Overview
    console.log('[Scene 2/5] Navigating to Cluster Overview (/app)...');
    await page.click('a[href="/app"]');
    await page.waitForURL('**/app');
    await page.waitForTimeout(2000);
    await page.screenshot({ path: path.join(OUTPUT_DIR, 'step2-cluster.png') });

    // SCENE 3: Submit Diamond DAG
    console.log('[Scene 3/5] Navigating to Submit (/app/submit)...');
    await page.click('a[href="/app/submit"]');
    await page.waitForURL('**/app/submit');
    await page.waitForTimeout(1000);
    
    // Select Diamond Pattern sample
    await page.selectOption('select', 'diamond');
    await page.waitForTimeout(1000);
    await page.screenshot({ path: path.join(OUTPUT_DIR, 'step3-submit-preview.png') });

    // Click Submit
    console.log('Submitting diamond workflow...');
    await page.click('button:has-text("SUBMIT WORKFLOW")');
    await page.waitForURL('**/app/workflows');
    await page.waitForTimeout(1500);

    // SCENE 4: DAG Visual Inspection
    console.log('[Scene 4/5] Inspecting Workflows Registry & Graph (/app/workflows)...');
    await page.screenshot({ path: path.join(OUTPUT_DIR, 'step4-workflows-registry.png') });
    
    // Open first workflow detail if available
    const detailBtn = page.locator('button:has-text("INSPECT GRAPH")').first();
    if (await detailBtn.isVisible()) {
      await detailBtn.click();
      await page.waitForTimeout(2000);
      await page.screenshot({ path: path.join(OUTPUT_DIR, 'step4-workflow-detail.png') });
    }

    // SCENE 5: Resilience Lab & Chaos Injection
    console.log('[Scene 5/5] Navigating to Resilience Lab (/app/resilience)...');
    await page.click('a[href="/app/resilience"]');
    await page.waitForURL('**/app/resilience');
    await page.waitForTimeout(1500);

    // Click Kill Leader
    console.log('Injecting fault: Kill Leader...');
    await page.click('button:has-text("KILL LEADER")');
    await page.waitForTimeout(500);
    await page.screenshot({ path: path.join(OUTPUT_DIR, 'step5-confirm-modal.png') });

    // Confirm in dialog
    await page.click('button:has-text("INJECT FAULT")');
    await page.waitForTimeout(3000); // Wait for failover narration and score line update
    await page.screenshot({ path: path.join(OUTPUT_DIR, 'step5-recovery-complete.png') });

    console.log('Demo recording sequence completed successfully!');
  } catch (err) {
    console.error('Error during demo recording:', err);
  } finally {
    await context.close();
    await browser.close();
  }
}

if (require.main === module) {
  runDemo().catch(console.error);
}

export { runDemo };
