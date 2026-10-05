local mp = require('mp')
local utils = require('mp.utils')
local manifest = utils.parse_json(manifest_json)
local options = manifest.options
local overlay = mp.create_osd_overlay('ass-events')
local segments, active, undo, pending = {}, nil, nil, false
local suppressed, bound, undo_bound = {}, false, false
local initial_playlist = nil
local last_pos = nil

local function clear_prompt()
    overlay:remove()
    if bound then mp.remove_key_binding('cue-skip'); bound = false end
end
local function valid(s, duration)
    return (s.kind == 'intro' or s.kind == 'outro') and type(s.start_ms) == 'number'
        and type(s.end_ms) == 'number' and s.start_ms >= 0 and s.end_ms > s.start_ms
        and duration and s.end_ms <= duration * 1000
end
local function skip()
    local pos = mp.get_property_number('time-pos')
    if not active or pending or not pos or pos * 1000 < active.start_ms or pos * 1000 >= active.end_ms then return end
    undo = pos
    suppressed[active] = true
    pending = true
    clear_prompt()
    mp.commandv('seek', tostring(active.end_ms / 1000), 'absolute', 'exact')
end
local function undo_skip()
    if undo then
        local pos = undo; undo = nil
        pending = true
        mp.commandv('seek', tostring(pos), 'absolute', 'exact')
    end
end
local function effective_key()
    -- User input.conf and forced bindings take precedence. Never advertise a key
    -- when another command wins, and don't replace it with a forced binding.
    local winner = nil
    for _,binding in ipairs(mp.get_property_native('input-bindings', {})) do
        if binding.key == options.key then
            if not winner or (binding.priority or 0) > (winner.priority or 0) then winner = binding end
        end
    end
    return winner and winner.cmd and winner.cmd:find('cue%-skip') ~= nil
end
local function update(_, pos)
    if type(pos) ~= 'number' then active = nil; clear_prompt(); return end
    if last_pos and pos < last_pos - 0.5 then
        for _,s in ipairs(segments) do
            if pos * 1000 >= s.start_ms and pos * 1000 < s.end_ms then suppressed[s] = true end
        end
    end
    last_pos = pos
    active = nil
    for _,s in ipairs(segments) do
        if options[s.kind] ~= 'off' and pos * 1000 >= s.start_ms and pos * 1000 < s.end_ms then active = s; break end
    end
    if not active or pending then clear_prompt(); return end
    if options[active.kind] == 'auto' and not suppressed[active] then skip(); return end
    if not bound then mp.add_key_binding(options.key, 'cue-skip', skip); bound = true end
    local label = effective_key() and ('Press ' .. options.key .. ' to skip ') or 'Skip available: '
    label = label .. active.kind
    local escaped = label:gsub('\\','\\e'):gsub('{','\\{'):gsub('}','\\}')
    overlay.data = '{\\an3\\pos(1260,650)\\fs26\\bord2\\shad1}' .. escaped
    overlay.res_x = 1280; overlay.res_y = 720
    overlay:update()
end
mp.register_event('start-file', function()
    clear_prompt(); segments = {}; active = nil; undo = nil; pending = false; suppressed = {}; last_pos = nil
    if undo_bound then mp.remove_key_binding('cue-undo-skip'); undo_bound = false end
end)
local function load_file()
    local playlist = mp.get_property_native('playlist', {})
    if not initial_playlist then initial_playlist = playlist end
    local index = (mp.get_property_number('playlist-pos', -1)) + 1
    local same = #playlist == #initial_playlist
    for i,p in ipairs(playlist) do if not initial_playlist[i] or p.filename ~= initial_playlist[i].filename then same = false end end
    local duration = mp.get_property_number('duration')
    segments = {}
    if same then
        for _,s in ipairs(manifest.entries[index] or {}) do if valid(s, duration) then segments[#segments+1] = s end end
    end
    if same and #segments == 0 then
        local path = manifest.files and manifest.files[index]
        local file = path and path ~= '' and io.open(path, 'r') or nil
        if file then
            local cached = utils.parse_json(file:read('*a')); file:close()
            if cached and cached.Version == manifest.version then
                for _,s in ipairs(cached.Segments or {}) do
                    if valid(s,duration) then segments[#segments+1] = s end
                end
            end
        end
    end
    local chapters = mp.get_property_native('chapter-list', {})
    if #segments == 0 then
        local aliases = {intro='intro', opening='intro', op='intro', outro='outro', ending='outro', ed='outro', credits='outro', ['end credits']='outro'}
        for i,c in ipairs(chapters) do
            local title = (c.title or ''):lower():match('^%s*(.-)%s*$')
            local kind = aliases[title]
            local ending = chapters[i+1] and chapters[i+1].time or duration
            if kind and c.time and ending then
                local s = {kind=kind, start_ms=c.time*1000, end_ms=ending*1000, origin='chapter'}
                if valid(s,duration) then segments[#segments+1] = s end
            end
        end
    end
    if #chapters == 0 and #segments > 0 and options.chapters_when_missing then
        local generated = {{time=0,title='Episode'}}
        for _,s in ipairs(segments) do
            if s.start_ms == 0 then generated[1].title=s.kind else generated[#generated+1]={time=s.start_ms/1000,title=s.kind} end
            if s.end_ms/1000 < duration then generated[#generated+1]={time=s.end_ms/1000,title='Episode'} end
        end
        table.sort(generated, function(a,b) return a.time < b.time end)
        local unique = {}
        for _,c in ipairs(generated) do if #unique > 0 and unique[#unique].time == c.time then unique[#unique]=c else unique[#unique+1]=c end end
        mp.set_property_native('chapter-list',unique)
    end
    mp.add_key_binding(options.undo_key, 'cue-undo-skip', undo_skip); undo_bound = true
    update(nil,mp.get_property_number('time-pos'))
end
mp.register_event('file-loaded',load_file)
-- Results completed by the startup worker become available without relaunching.
mp.add_periodic_timer(2, function()
    if #segments == 0 and mp.get_property_number('duration') then load_file() end
end)
mp.register_event('playback-restart', function() pending = false; update(nil,mp.get_property_number('time-pos')) end)
mp.observe_property('time-pos','number',update)
mp.register_event('shutdown',clear_prompt)
