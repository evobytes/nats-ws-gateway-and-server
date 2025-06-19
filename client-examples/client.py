import asyncio
import websockets
import json
from datetime import datetime

async def clock_client():
    uri = "wss://myserver.domain/hive-ws/"
    async with websockets.connect(uri) as ws:
        async def recv():
            async for message in ws:
                try:
                    msg = json.loads(message)
                    print(f"Received: {msg}")
                except Exception as e:
                    print(f"Decode error: {e}")

        async def send_clock():
            while True:
                now = datetime.utcnow().isoformat()
                await ws.send(json.dumps({"type": "clock", "data": now}))
                await asyncio.sleep(60)

        await asyncio.gather(recv(), send_clock())

asyncio.run(clock_client())

