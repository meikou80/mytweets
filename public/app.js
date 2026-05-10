window.addEventListener('DOMContentLoaded', function() {
  var form = document.querySelector('#form');
  var settingsForm = document.querySelector('#settings');
  var ul = document.querySelector('#tweets');
  var status = document.querySelector('#status');
  var searchButton = document.querySelector('#search');
  var exportButton = document.querySelector('#export');
  var refreshButton = document.querySelector('#refresh');
  var saveConfigButton = document.querySelector('#save-config');

  function setStatus(message, isError) {
    status.textContent = message || '';
    status.className = isError ? 'error' : '';
  }

  function fetchJSON(url, options) {
    return window.fetch(url, options).then(function(response) {
      return response.json().then(function(body) {
        if (!response.ok) {
          throw new Error(body.error || response.statusText);
        }
        return body;
      });
    });
  }

  function linesToArray(value) {
    return value.split(/\r?\n/).map(function(line) {
      return line.trim();
    }).filter(function(line) {
      return line !== '';
    });
  }

  function arrayToLines(values) {
    return (values || []).join('\n');
  }

  function numberValue(selector) {
    var value = document.querySelector(selector).value;
    if (value === '') {
      return 0;
    }
    return Number(value);
  }

  function fillConfigForm(cfg) {
    document.querySelector('#cfg-users').value = arrayToLines(cfg.users);
    document.querySelector('#cfg-keywords').value = arrayToLines(cfg.keywords);
    document.querySelector('#cfg-keyword-search-mode').value = cfg.keyword_search_mode || 'separate';
    document.querySelector('#cfg-search-endpoint').value = cfg.search_endpoint || 'recent';
    document.querySelector('#cfg-lookback-days').value = cfg.lookback_days || '';
    document.querySelector('#cfg-start-time').value = cfg.start_time || '';
    document.querySelector('#cfg-end-time').value = cfg.end_time || '';
    document.querySelector('#cfg-language').value = cfg.language || '';
    document.querySelector('#cfg-max-pages').value = cfg.max_pages_per_refresh || 1;
    document.querySelector('#cfg-exclude-reposts').checked = !!cfg.exclude_reposts;
    document.querySelector('#cfg-exclude-replies').checked = !!cfg.exclude_replies;
  }

  function readConfigForm() {
    return {
      users: linesToArray(document.querySelector('#cfg-users').value),
      keywords: linesToArray(document.querySelector('#cfg-keywords').value),
      keyword_search_mode: document.querySelector('#cfg-keyword-search-mode').value,
      search_endpoint: document.querySelector('#cfg-search-endpoint').value,
      lookback_days: numberValue('#cfg-lookback-days'),
      start_time: document.querySelector('#cfg-start-time').value.trim(),
      end_time: document.querySelector('#cfg-end-time').value.trim(),
      language: document.querySelector('#cfg-language').value.trim(),
      exclude_reposts: document.querySelector('#cfg-exclude-reposts').checked,
      exclude_replies: document.querySelector('#cfg-exclude-replies').checked,
      max_pages_per_refresh: numberValue('#cfg-max-pages')
    };
  }

  function loadConfig() {
    fetchJSON('/config').then(function(cfg) {
      fillConfigForm(cfg);
    }).catch(function(error) {
      setStatus(error.message, true);
    });
  }

  function saveConfig() {
    saveConfigButton.disabled = true;
    setStatus('設定を保存しています...', false);
    fetchJSON('/config', {
      method: 'PUT',
      headers: {'content-type': 'application/json'},
      body: JSON.stringify(readConfigForm())
    }).then(function(cfg) {
      fillConfigForm(cfg);
      setStatus('設定を保存しました', false);
    }).catch(function(error) {
      setStatus(error.message, true);
    }).finally(function() {
      saveConfigButton.disabled = false;
    });
  }

  function update(prefixMessage, isPrefixError) {
    if (!prefixMessage) {
      setStatus('検索しています...', false);
    }
    ul.innerHTML = '';
    fetchJSON('/search', {
      method: 'POST',
      body: new FormData(form)
    }).then(function(posts) {
      var message = posts.length + '件見つかりました';
      if (prefixMessage) {
        message = prefixMessage + ' / 表示 ' + message;
      }
      setStatus(message, !!isPrefixError);
      for (var post of posts) {
        var a = document.createElement('a');
        a.textContent = post.text;
        a.href = post.url;
        a.target = '_blank';
        a.rel = 'noopener noreferrer';

        var meta = document.createElement('div');
        meta.className = 'meta';
        meta.textContent = '@' + (post.username || 'unknown') + ' / ' + (post.created_at || '');

        var li = document.createElement('li');
        li.appendChild(meta);
        li.appendChild(a);
        ul.appendChild(li);
      }
    }).catch(function(error) {
      setStatus(error.message, true);
    });
  }

  function refresh() {
    refreshButton.disabled = true;
    setStatus('Xから取得しています...', false);
    fetchJSON('/refresh', {
      method: 'POST'
    }).then(function(result) {
      var message = '取得完了: API取得 ' + result.fetched + '件 / 追加 ' + result.inserted + '件 / 更新 ' + result.updated + '件 / ソース ' + result.sources + '件';
      if (result.errors && result.errors.length > 0) {
        message += ' / エラー ' + result.errors.length + '件';
        if (result.errors[0].error) {
          message += ' / ' + result.errors[0].error;
        }
      }
      update(message, result.errors && result.errors.length > 0);
    }).catch(function(error) {
      setStatus(error.message, true);
    }).finally(function() {
      refreshButton.disabled = false;
    });
  }

  function exportCSV() {
    var params = new URLSearchParams(new FormData(form));
    window.location.href = '/export.csv?' + params.toString();
  }

  document.querySelector('#q').addEventListener('keydown', function(e) {
    if (e.key === 'Enter' || e.which === 13) {
      e.preventDefault();
      update();
    }
  });
  settingsForm.addEventListener('submit', function(e) {
    e.preventDefault();
    saveConfig();
  });
  searchButton.addEventListener('click', update, false);
  exportButton.addEventListener('click', exportCSV, false);
  refreshButton.addEventListener('click', refresh, false);
  saveConfigButton.addEventListener('click', saveConfig, false);
  loadConfig();
}, false);
