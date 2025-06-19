using System.Net.WebSockets;
using System.Text;
using System.Text.Json;

var uri = new Uri("wss://myserver.domain/hive-ws/");
using var client = new ClientWebSocket();
await client.ConnectAsync(uri, CancellationToken.None);

_ = Task.Run(async () =>
{
    var buffer = new byte[2048];
    while (true)
    {
        var result = await client.ReceiveAsync(buffer, CancellationToken.None);
        var json = Encoding.UTF8.GetString(buffer, 0, result.Count);
        Console.WriteLine($"Received: {json}");
    }
});

while (true)
{
    var payload = new
    {
        type = "clock",
        data = DateTime.UtcNow.ToString("o")
    };
    var json = JsonSerializer.Serialize(payload);
    var bytes = Encoding.UTF8.GetBytes(json);
    await client.SendAsync(bytes, WebSocketMessageType.Text, true, CancellationToken.None);
    await Task.Delay(TimeSpan.FromMinutes(1));
}

