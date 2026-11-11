Stop-Process -Name msedge -Force -ErrorAction SilentlyContinue
$edge = "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
$outDir = "C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots"
$artDir = "C:\Users\OM WATTAMWAR\.gemini\antigravity-cli\brain\59670479-638f-4b8e-af82-434145808c5f"

Write-Host "Capturing Workflows 1440..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-50"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint5-poster-workflows-1440.png"', '--window-size=1440,1200', '"http://localhost:3000/app/workflows"' -Wait

Write-Host "Capturing Workflows 390..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-51"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint5-poster-workflows-390.png"', '--window-size=390,900', '"http://localhost:3000/app/workflows"' -Wait

Write-Host "Capturing Workflows Full 1440..."
Start-Process -FilePath $edge -ArgumentList '--headless=new', '--disable-gpu', '--user-data-dir="C:\Users\OM WATTAMWAR\AppData\Local\Temp\edge-ss-52"', '--screenshot="C:\Users\OM WATTAMWAR\Desktop\distributed-scheduler\docs\screenshots\checkpoint5-poster-workflows-full-1440.png"', '--window-size=1440,2200', '"http://localhost:3000/app/workflows"' -Wait

Copy-Item "$outDir\checkpoint5-poster-*.png" -Destination "$artDir\" -Force
Get-ChildItem "$artDir\checkpoint5-poster-*.png" | Format-Table Name, Length
