window.addEventListener('DOMContentLoaded', function() {
  var form = document.querySelector('#form');
  var ul = document.querySelector('#tweets');
  var status = document.querySelector('#status');
  var searchButton = document.querySelector('#search');
  var exportButton = document.querySelector('#export');
  var refreshButton = document.querySelector('#refresh');

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
  searchButton.addEventListener('click', update, false);
  exportButton.addEventListener('click', exportCSV, false);
  refreshButton.addEventListener('click', refresh, false);
}, false);
