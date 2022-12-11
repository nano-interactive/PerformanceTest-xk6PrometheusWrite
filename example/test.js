import http from 'k6/http';

export const options = {
  vus: 2,
  duration: '10s',
};

export default function () {
  //http.get('http://localhost:8080?rnd=' + Math.random()*100 + "&err=1");
  http.get('http://localhost:8080?rnd=' + Math.random()*100);
}
