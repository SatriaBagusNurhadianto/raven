using System;
using System.Net.Http;
using System.Text;
using System.Text.Json;
using System.Threading.Tasks;

namespace Raven {
    public class RavenClient {
        private readonly HttpClient http;
        public string BaseUrl { get; set; } = "http://localhost:8080";
        public string GameId { get; set; }
        public string AccessToken { get; set; }
        public string RefreshToken { get; set; }

        public RavenClient(string baseUrl, string gameId) {
            BaseUrl = baseUrl.TrimEnd('/');
            GameId = gameId;
            http = new HttpClient();
        }

        private async Task<JsonDocument> RequestAsync(string method, string path, object body = null) {
            var req = new HttpRequestMessage(new HttpMethod(method), BaseUrl + path);
            if (AccessToken != null) req.Headers.Authorization = new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", AccessToken);
            if (body != null) req.Content = new StringContent(JsonSerializer.Serialize(body), Encoding.UTF8, "application/json");
            var res = await http.SendAsync(req);
            var text = await res.Content.ReadAsStringAsync();
            if (!res.IsSuccessStatusCode) throw new Exception($"HTTP {(int)res.StatusCode}: {text}");
            if (string.IsNullOrWhiteSpace(text)) return null;
            return JsonDocument.Parse(text);
        }

        public Task<JsonDocument> GuestAsync(string deviceId) =>
            RequestAsync("POST", "/v1/auth/guest", new { game_id = GameId, device_id = deviceId });

        public Task<JsonDocument> GetSaveAsync(string slot) =>
            RequestAsync("GET", $"/v1/saves/{UriEncode(slot)}");

        public Task<JsonDocument> PutSaveAsync(string slot, object data) =>
            RequestAsync("PUT", $"/v1/saves/{UriEncode(slot)}", new { data });

        public Task<JsonDocument> SubmitScoreAsync(string boardId, double score) =>
            RequestAsync("POST", $"/v1/leaderboards/{boardId}/scores", new { score });

        private static string UriEncode(string s) => Uri.EscapeDataString(s);
    }
}
