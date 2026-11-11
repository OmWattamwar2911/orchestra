from PIL import Image
import os

files = [
    'docs/demo-recording/demo-scene1-landing.png',
    'docs/demo-recording/demo-scene2-cluster.png',
    'docs/demo-recording/demo-scene3-submit.png',
    'docs/demo-recording/demo-scene4-workflows.png',
    'docs/demo-recording/demo-scene5-resilience.png'
]

images = []
for f in files:
    if os.path.exists(f):
        im = Image.open(f)
        im_resized = im.resize((960, 600), Image.Resampling.LANCZOS).convert('RGB')
        images.append(im_resized)

if images:
    out_path = 'docs/demo.gif'
    images[0].save(
        out_path,
        save_all=True,
        append_images=images[1:],
        duration=2200,
        loop=0,
        optimize=True
    )
    print(f"Generated {out_path} ({os.path.getsize(out_path)} bytes)")
else:
    print("No images found")
