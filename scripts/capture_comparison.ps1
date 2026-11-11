Stop-Process -Name msedge -Force -ErrorAction SilentlyContinue
$edge = "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
$outDir = "C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots"
$artDir = "C:\Users\OM WATTAMWAR\.gemini\antigravity-cli\brain\59670479-638f-4b8e-af82-434145808c5f"

Write-Host "Capturing Docker Container Landing (http://localhost:3000/)..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-comp-1"', "--screenshot=`"$outDir\docker-landing-1440.png`"", '--window-size=1440,900', '"http://localhost:3000/"' -Wait

Write-Host "Capturing Docker Container Dashboard (http://localhost:3000/app)..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-comp-2"', "--screenshot=`"$outDir\docker-cluster-1440.png`"", '--window-size=1440,900', '"http://localhost:3000/app"' -Wait

Write-Host "Capturing Vite Dev Server Landing (http://127.0.0.1:3000/)..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-comp-3"', "--screenshot=`"$outDir\vite-landing-1440.png`"", '--window-size=1440,900', '"http://127.0.0.1:3000/"' -Wait

Write-Host "Capturing Vite Dev Server Dashboard (http://127.0.0.1:3000/app)..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-comp-4"', "--screenshot=`"$outDir\vite-cluster-1440.png`"", '--window-size=1440,900', '"http://127.0.0.1:3000/app"' -Wait

Copy-Item "$outDir\docker-*.png" -Destination "$artDir\" -Force
Copy-Item "$outDir\vite-*.png" -Destination "$artDir\" -Force
Get-ChildItem "$artDir\docker-*.png", "$artDir\vite-*.png" | Format-Table Name, Length
Write-Host "Comparison capture complete!"
