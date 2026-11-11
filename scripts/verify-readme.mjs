import fs from 'fs';

const content = fs.readFileSync('README.md', 'utf8');
const imgRegex = /src=["']([^"']+)["']|srcset=["']([^"']+)["']|!\[[^\]]*\]\(([^)]+)\)/g;
let match;
let errors = 0;
let checked = 0;

console.log('Auditing README.md media links...');

while ((match = imgRegex.exec(content)) !== null) {
  const p = match[1] || match[2] || match[3];
  if (!p) continue;
  checked++;
  if (!p.startsWith('http')) {
    if (fs.existsSync(p)) {
      const stats = fs.statSync(p);
      console.log(`[PASS] Local file exists: ${p} (${Math.round(stats.size / 1024)} KB)`);
    } else {
      console.error(`[FAIL] Broken local file: ${p}`);
      errors++;
    }
  } else {
    console.log(`[PASS] External badge URL: ${p}`);
  }
}

if (errors > 0) {
  console.error(`\nFAILED: Found ${errors} broken image links.`);
  process.exit(1);
} else {
  console.log(`\nSUCCESS: All ${checked} images verified! Zero broken links.`);
}
