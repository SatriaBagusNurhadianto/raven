# Raven Godot SDK (GDScript)
extends Node

var base_url: String = "http://localhost:8080"
var game_id: String = ""
var access_token: String = ""
var refresh_token: String = ""

func _request(method: String, path: String, body: Dictionary = {}) -> Dictionary:
	var http := HTTPRequest.new()
	add_child(http)
	var url := base_url.rstrip("/") + path
	var headers := ["Content-Type: application/json"]
	if access_token != "":
		headers.append("Authorization: Bearer " + access_token)
	var err = http.request(url, headers, method, JSON.stringify(body) if body.size() > 0 else "")
	var result = await http.request_completed
	http.queue_free()
	if result[0] != OK:
		return {"error": "request failed"}
	var response = JSON.parse_string(result[3].get_string_from_utf8())
	return response if response != null else {}

func guest(device_id: String) -> Dictionary:
	return await _request("POST", "/v1/auth/guest", {"game_id": game_id, "device_id": device_id})

func get_save(slot: String) -> Dictionary:
	return await _request("GET", "/v1/saves/" + slot.uri_encode())

func put_save(slot: String, data: Dictionary) -> Dictionary:
	return await _request("PUT", "/v1/saves/" + slot.uri_encode(), {"data": data})

func submit_score(board_id: String, score: float) -> Dictionary:
	return await _request("POST", "/v1/leaderboards/" + board_id + "/scores", {"score": score})
