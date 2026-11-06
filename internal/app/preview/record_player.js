const video = document.querySelector('video');
const stage = document.querySelector('#stage');
const seek = document.querySelector('#seek');
const play = document.querySelector('#play');
const playShape = document.querySelector('#play-shape');
const clock = document.querySelector('#clock');
const fullscreen = document.querySelector('#fullscreen');
const statusLine = document.querySelector('#status');
const sound = document.querySelector('#sound');
const soundShape = document.querySelector('#sound-shape');
const volume = document.querySelector('#volume');
const closeConfirmation = document.querySelector('#close-confirmation');
window.screenpinConfirmClose = () => {
  video.pause();
  if (!closeConfirmation.open) closeConfirmation.showModal();
};
function keepPreview() {
  closeConfirmation.close();
  location.assign('keep-preview');
}
document.querySelector('#keep-preview').addEventListener('click', keepPreview);
document.querySelector('#discard-preview').addEventListener('click', () => location.assign('discard-preview'));
closeConfirmation.addEventListener('cancel', event => {
  event.preventDefault();
  keepPreview();
});

document.querySelector('#audio-description').textContent = document.documentElement.dataset.audio || '无音轨';

function updateSound() {
  const muted = video.muted || video.volume === 0;
  const label = muted ? '取消静音' : '静音';
  sound.title = label;
  sound.setAttribute('aria-label', label);
  sound.setAttribute('aria-pressed', String(muted));
  soundShape.setAttribute('d', muted ? 'm10 6 4 4m0-4-4 4' : 'M10.5 6a3 3 0 0 1 0 4m2-6a5.7 5.7 0 0 1 0 8');
  volume.value = video.muted ? 0 : video.volume;
}

sound.addEventListener('click', () => {
  if (video.muted || video.volume === 0) {
    if (video.volume === 0) video.volume = 0.5;
    video.muted = false;
  } else {
    video.muted = true;
  }
});
volume.addEventListener('input', () => {
  video.volume = Number(volume.value);
  video.muted = false;
});
video.addEventListener('volumechange', updateSound);

function timecode(seconds) {
  const total = Number.isFinite(seconds) ? Math.floor(seconds) : 0;
  const minutes = String(Math.floor(total / 60)).padStart(2, '0');
  return `${minutes}:${String(total % 60).padStart(2, '0')}`;
}

function updateTime() {
  if (Number.isFinite(video.duration)) seek.max = video.duration;
  seek.value = video.currentTime;
  const progress = video.duration > 0 ? (100 * video.currentTime / video.duration) : 0;
  seek.style.setProperty('--progress', `${progress}%`);
  clock.textContent = `${timecode(video.currentTime)} / ${timecode(video.duration)}`;
  seek.setAttribute('aria-valuetext', clock.textContent);
}

function updatePlay() {
  const label = video.paused ? '播放' : '暂停';
  play.title = label;
  play.setAttribute('aria-label', label);
  playShape.setAttribute('d', video.paused ? 'M5 3l8 5-8 5z' : 'M6 3v10m4-10v10');
}

function togglePlayback() {
  if (video.paused) {
    if (video.currentTime < start || video.currentTime >= (end ?? video.duration)) video.currentTime = start;
    video.play().catch(() => { statusLine.textContent = '预览无法播放，仍可保存 MP4'; });
  } else {
    video.pause();
  }
}

function updateMetadata() {
  if (video.videoWidth > 0) {
    document.querySelector('#resolution').textContent = `${video.videoWidth} × ${video.videoHeight}`;
  }
  updateTime();
}

play.addEventListener('click', togglePlayback);
video.addEventListener('click', togglePlayback);
video.addEventListener('loadedmetadata', updateMetadata);
video.addEventListener('durationchange', updateTime);
video.addEventListener('timeupdate', updateTime);
video.addEventListener('play', updatePlay);
video.addEventListener('pause', updatePlay);
seek.addEventListener('input', () => {
  if (Number.isFinite(video.duration)) video.currentTime = Number(seek.value);
  updateTime();
});
document.querySelector('#rate').addEventListener('change', event => {
  video.playbackRate = Number(event.target.value);
});
fullscreen.addEventListener('click', () => {
  const change = document.fullscreenElement ? document.exitFullscreen() : stage.requestFullscreen();
  change.catch(() => {});
});
document.addEventListener('fullscreenchange', () => {
  const label = document.fullscreenElement ? '退出全屏' : '全屏';
  fullscreen.title = label;
  fullscreen.setAttribute('aria-label', label);
});
document.addEventListener('keydown', event => {
  if (event.code === 'Space' && !event.target.closest('a, button, input, select')) {
    event.preventDefault();
    togglePlayback();
  }
});
video.controls = false;
seek.hidden = false;
document.querySelector('.player-controls').hidden = false;
updateMetadata();
updatePlay();
updateSound();

const format = document.querySelector('#export-format');
const inButton = document.querySelector('#mark-in');
const outButton = document.querySelector('#mark-out');
const reset = document.querySelector('#reset-trim');
let start = 0, end = null;
function updateEdit() {
  const duration = video.duration;
  inButton.disabled = outButton.disabled = !Number.isFinite(duration) || duration < .05;
  reset.disabled = start === 0 && end === null;
  seek.classList.toggle('trimmed', !reset.disabled);
  if (duration > 0) {
    seek.style.setProperty('--start', 100 * start / duration + '%');
    seek.style.setProperty('--end', 100 * (end ?? duration) / duration + '%');
  }
  const label = t => timecode(t) + '.' + String(Math.floor(t * 1000) % 1000).padStart(3, '0');
  document.querySelector('#trim-label').textContent = reset.disabled ? '保留整段' :
    '保留 ' + label(start) + ' – ' + label(end ?? duration);
  const params = new URLSearchParams();
  if (start) params.set('start', start.toFixed(3));
  if (end !== null) params.set('end', end.toFixed(3));
  if (format.value !== 'mp4') params.set('format', format.value);
  const query = params.size ? '?' + params.toString() : '';
  const save = document.querySelector('#save');
  save.textContent = format.value === 'mp4' ? '保存 MP4' : '导出 ' + format.selectedOptions[0].text;
  save.href = 'save' + query;
  const saveAs = document.querySelector('#save-as');
  saveAs.href = 'save-as' + query;
  saveAs.hidden = format.value !== 'mp4';
}
inButton.onclick = () => {
  video.pause();
  if ((end ?? video.duration) - video.currentTime < .05) {
    statusLine.textContent = '起点须早于终点至少 0.05 秒';
    return;
  }
  start = video.currentTime;
  updateEdit();
};
outButton.onclick = () => {
  video.pause();
  if (video.currentTime - start < .05) {
    statusLine.textContent = '终点须晚于起点至少 0.05 秒';
    return;
  }
  end = video.currentTime;
  updateEdit();
};
reset.onclick = () => { start = 0; end = null; updateEdit(); };
format.onchange = updateEdit;
video.addEventListener('durationchange', updateEdit);
video.addEventListener('timeupdate', () => {
  if (!video.paused && end !== null && video.currentTime >= end) {
    video.pause();
    video.currentTime = end;
  }
});
updateEdit();
