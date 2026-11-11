Stop-Process -Name msedge -Force -ErrorAction SilentlyContinue
$edge = "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
$outDir = "C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots"
$artDir = "C:\Users\OM WATTAMWAR\.gemini\antigravity-cli\brain\59670479-638f-4b8e-af82-434145808c5f"

Write-Host "Capturing Docs 1440..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-80"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint8-poster-docs-1440.png"', '--window-size=1440,1100', '"http://localhost:3000/app/docs"' -Wait

Write-Host "Capturing Docs 390..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-81"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint8-poster-docs-390.png"', '--window-size=390,1100', '"http://localhost:3000/app/docs"' -Wait

Write-Host "Capturing Docs Full 1440..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-82"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint8-poster-docs-full-1440.png"', '--window-size=1440,2800', '"http://localhost:3000/app/docs"' -Wait

Copy-Item "$outDir\checkpoint8-poster-*.png" -Destination "$artDir\" -Force
Get-ChildItem "$artDir\checkpoint8-poster-*.png" | Format-Table Name, Length
